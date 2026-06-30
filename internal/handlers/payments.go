package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/account"
	"github.com/stripe/stripe-go/v82/accountlink"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/transfer"
	"github.com/stripe/stripe-go/v82/webhook"
)

type PaymentHandler struct {
	DB                  *sqlx.DB
	StripeWebhookSecret string
}

type createPaymentRequest struct {
	BookingID string `json:"bookingId"`
}

type paymentResponse struct {
	PaymentID    string `json:"paymentId"`
	ClientSecret string `json:"clientSecret"`
	AmountCents  int    `json:"amountCents"`
	Currency     string `json:"currency"`
}

type connectOnboardingRequest struct {
	ReturnURL  string `json:"returnUrl"`
	RefreshURL string `json:"refreshUrl"`
	Country    string `json:"country"`
}

type connectOnboardingResponse struct {
	AccountID string `json:"accountId"`
	URL       string `json:"url"`
}

type payoutStatusResponse struct {
	AccountID        *string `json:"accountId"`
	ChargesEnabled   bool    `json:"chargesEnabled"`
	PayoutsEnabled   bool    `json:"payoutsEnabled"`
	DetailsSubmitted bool    `json:"detailsSubmitted"`
}

type releasePayoutResponse struct {
	PayoutID         string `json:"payoutId"`
	StripeTransferID string `json:"stripeTransferId"`
	AmountCents      int    `json:"amountCents"`
	PlatformFeeCents int    `json:"platformFeeCents"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
}

// Lifetime membership ($100 one-time)
type createLifetimeRequest struct {
	// optional metadata
}

type lifetimeStatus struct {
	HasMembership        bool   `json:"hasMembership"`
	Tier                 string `json:"tier"`
	AmountCents          int    `json:"amountCents"`
	PaidAt               string `json:"paidAt,omitempty"`
	FreeExploreUsed      int    `json:"freeExploreUsed"` // 0 or 1 today
	FreeExploreRemaining int    `json:"freeExploreRemaining"`
}

// CreatePaymentIntent creates a Stripe PaymentIntent for a booking.
// POST /payments/create
func (h *PaymentHandler) CreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req createPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.BookingID == "" {
		writeError(w, http.StatusBadRequest, "bookingId is required")
		return
	}

	// Get booking
	var booking models.Booking
	err := h.DB.Get(&booking, "SELECT * FROM bookings WHERE id = ? AND user_id = ?", req.BookingID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Booking not found")
		return
	}

	if booking.PaymentStatus == "paid" {
		writeError(w, http.StatusBadRequest, "Booking is already paid")
		return
	}

	// Get experience for pricing
	var exp models.Experience
	err = h.DB.Get(&exp, "SELECT * FROM experiences WHERE id = ?", booking.ExperienceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Experience not found")
		return
	}

	if exp.PriceUSD == nil {
		writeError(w, http.StatusBadRequest, "Experience has no USD price set")
		return
	}

	guestCount := 1
	if booking.GuestCount != nil {
		guestCount = *booking.GuestCount
	}
	amountCents := int(*exp.PriceUSD*100) * guestCount

	// Get or create Stripe customer
	stripeCustomerID, err := h.getOrCreateCustomer(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create payment customer")
		return
	}

	// Create PaymentIntent
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(int64(amountCents)),
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		Customer: stripe.String(stripeCustomerID),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
		Metadata: map[string]string{
			"booking_id": req.BookingID,
			"user_id":    userID,
			"type":       "marketplace_booking",
		},
		TransferGroup: stripe.String("booking_" + req.BookingID),
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create payment: %v", err))
		return
	}

	// Record payment in DB
	now := time.Now()
	payment := models.Payment{
		ID:                    uuid.New().String(),
		UserID:                userID,
		BookingID:             &req.BookingID,
		StripePaymentIntentID: &pi.ID,
		StripeCustomerID:      &stripeCustomerID,
		AmountCents:           amountCents,
		Currency:              "usd",
		Status:                "pending",
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	_, err = h.DB.Exec(`INSERT INTO payments
		(id, user_id, booking_id, stripe_payment_intent_id, stripe_customer_id, amount_cents, currency, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		payment.ID, payment.UserID, payment.BookingID, payment.StripePaymentIntentID,
		payment.StripeCustomerID, payment.AmountCents, payment.Currency, payment.Status,
		payment.CreatedAt, payment.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to record payment")
		return
	}

	// Link payment to booking
	h.DB.Exec("UPDATE bookings SET stripe_payment_id = ?, total_usd_cents = ?, payout_status = 'not_ready' WHERE id = ?",
		payment.ID, amountCents, req.BookingID)

	writeJSON(w, http.StatusCreated, paymentResponse{
		PaymentID:    payment.ID,
		ClientSecret: pi.ClientSecret,
		AmountCents:  amountCents,
		Currency:     "usd",
	})
}

// GetPayment returns payment details.
// GET /payments/{id}
func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	paymentID := chi.URLParam(r, "id")

	var payment models.Payment
	err := h.DB.Get(&payment, "SELECT * FROM payments WHERE id = ? AND user_id = ?", paymentID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Payment not found")
		return
	}

	writeJSON(w, http.StatusOK, payment)
}

// ListMyPayments returns all payments for the current user.
// GET /payments
func (h *PaymentHandler) ListMyPayments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var payments []models.Payment
	err := h.DB.Select(&payments, "SELECT * FROM payments WHERE user_id = ? ORDER BY created_at DESC", userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch payments")
		return
	}
	if payments == nil {
		payments = []models.Payment{}
	}
	writeJSON(w, http.StatusOK, payments)
}

// CreateConnectOnboardingLink creates/reuses a Stripe Express account and returns
// an onboarding link for hosts and workers who can receive marketplace payouts.
// POST /payments/connect/onboarding
func (h *PaymentHandler) CreateConnectOnboardingLink(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req connectOnboardingRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ReturnURL == "" {
		req.ReturnURL = "https://empowertours.xyz/payouts/return"
	}
	if req.RefreshURL == "" {
		req.RefreshURL = "https://empowertours.xyz/payouts/refresh"
	}
	if req.Country == "" {
		req.Country = "US"
	}

	accountID, err := h.getOrCreateConnectedAccount(userID, req.Country)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	link, err := accountlink.New(&stripe.AccountLinkParams{
		Account:    stripe.String(accountID),
		RefreshURL: stripe.String(req.RefreshURL),
		ReturnURL:  stripe.String(req.ReturnURL),
		Type:       stripe.String("account_onboarding"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create onboarding link: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, connectOnboardingResponse{AccountID: accountID, URL: link.URL})
}

// GetPayoutStatus refreshes and returns the user's Stripe Connect payout state.
// GET /payments/payout-status
func (h *PaymentHandler) GetPayoutStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var status payoutStatusResponse
	err := h.DB.Get(&status, `SELECT stripe_account_id AS account_id,
		stripe_charges_enabled AS charges_enabled,
		stripe_payouts_enabled AS payouts_enabled,
		stripe_details_submitted AS details_submitted
		FROM users WHERE id = ?`, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	if status.AccountID != nil && *status.AccountID != "" && stripe.Key != "" {
		acct, err := account.GetByID(*status.AccountID, nil)
		if err == nil {
			status.ChargesEnabled = acct.ChargesEnabled
			status.PayoutsEnabled = acct.PayoutsEnabled
			status.DetailsSubmitted = acct.DetailsSubmitted
			_, _ = h.DB.Exec(`UPDATE users SET stripe_charges_enabled = ?, stripe_payouts_enabled = ?, stripe_details_submitted = ? WHERE id = ?`,
				status.ChargesEnabled, status.PayoutsEnabled, status.DetailsSubmitted, userID)
		}
	}

	writeJSON(w, http.StatusOK, status)
}

// ReleaseBookingPayout transfers booking funds to the experience creator's
// Stripe Connect account. Admins can release any booking; creators can release
// payouts for their own paid and completed bookings.
// POST /payments/bookings/{id}/release
func (h *PaymentHandler) ReleaseBookingPayout(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	bookingID := chi.URLParam(r, "id")

	type bookingPaymentRow struct {
		BookingID        string  `db:"booking_id"`
		BookingStatus    string  `db:"booking_status"`
		PaymentStatus    string  `db:"payment_status"`
		CreatorID        *string `db:"creator_id"`
		StripeAccountID  *string `db:"stripe_account_id"`
		PayoutsEnabled   bool    `db:"stripe_payouts_enabled"`
		PaymentID        *string `db:"payment_id"`
		AmountCents      *int    `db:"amount_cents"`
		Currency         *string `db:"currency"`
		ExistingTransfer *string `db:"stripe_transfer_id"`
	}

	var row bookingPaymentRow
	err := h.DB.Get(&row, `SELECT
			b.id AS booking_id,
			b.status AS booking_status,
			b.payment_status AS payment_status,
			b.stripe_transfer_id AS stripe_transfer_id,
			e.creator_id AS creator_id,
			u.stripe_account_id AS stripe_account_id,
			u.stripe_payouts_enabled AS stripe_payouts_enabled,
			p.id AS payment_id,
			p.amount_cents AS amount_cents,
			p.currency AS currency
		FROM bookings b
		LEFT JOIN experiences e ON e.id = b.experience_id
		LEFT JOIN users u ON u.id = e.creator_id
		LEFT JOIN payments p ON p.id = b.stripe_payment_id
		WHERE b.id = ?`, bookingID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Booking not found")
		return
	}

	if !middleware.IsAdmin(r.Context()) && (row.CreatorID == nil || *row.CreatorID != userID) {
		writeError(w, http.StatusForbidden, "Only the experience creator or an admin can release this payout")
		return
	}
	if row.PaymentStatus != "paid" {
		writeError(w, http.StatusBadRequest, "Booking must be paid before payout")
		return
	}
	if row.ExistingTransfer != nil && *row.ExistingTransfer != "" {
		writeError(w, http.StatusBadRequest, "Payout was already released")
		return
	}
	if row.StripeAccountID == nil || *row.StripeAccountID == "" || !row.PayoutsEnabled {
		writeError(w, http.StatusBadRequest, "Provider payout account is not ready")
		return
	}
	if row.AmountCents == nil || *row.AmountCents <= 0 {
		writeError(w, http.StatusBadRequest, "No payment amount found for booking")
		return
	}

	currency := "usd"
	if row.Currency != nil && *row.Currency != "" {
		currency = *row.Currency
	}
	platformFee := int(float64(*row.AmountCents) * 0.10)
	transferAmount := *row.AmountCents - platformFee
	description := "EmpowerTours payout for booking " + bookingID
	tr, err := transfer.New(&stripe.TransferParams{
		Amount:        stripe.Int64(int64(transferAmount)),
		Currency:      stripe.String(currency),
		Destination:   stripe.String(*row.StripeAccountID),
		Description:   stripe.String(description),
		TransferGroup: stripe.String("booking_" + bookingID),
		Metadata: map[string]string{
			"booking_id": bookingID,
			"type":       "marketplace_payout",
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to release payout: %v", err))
		return
	}

	payoutID := uuid.New().String()
	now := time.Now()
	_, err = h.DB.Exec(`INSERT INTO payouts
		(id, booking_id, provider_user_id, payment_id, stripe_transfer_id, amount_cents, platform_fee_cents, currency, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'paid', ?, ?)`,
		payoutID, bookingID, *row.CreatorID, row.PaymentID, tr.ID, transferAmount, platformFee, currency, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to record payout")
		return
	}

	_, _ = h.DB.Exec(`UPDATE bookings SET payout_status = 'paid', stripe_transfer_id = ?, platform_fee_cents = ? WHERE id = ?`,
		tr.ID, platformFee, bookingID)

	writeJSON(w, http.StatusCreated, releasePayoutResponse{
		PayoutID:         payoutID,
		StripeTransferID: tr.ID,
		AmountCents:      transferAmount,
		PlatformFeeCents: platformFee,
		Currency:         currency,
		Status:           "paid",
	})
}

// HandleWebhook processes Stripe webhook events.
// POST /webhooks/stripe
func (h *PaymentHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to read body")
		return
	}

	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), h.StripeWebhookSecret)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid webhook signature")
		return
	}

	switch event.Type {
	case "payment_intent.succeeded":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			writeError(w, http.StatusBadRequest, "Failed to parse event")
			return
		}
		h.handlePaymentSuccess(pi.ID)

	case "payment_intent.payment_failed":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			writeError(w, http.StatusBadRequest, "Failed to parse event")
			return
		}
		h.handlePaymentFailure(pi.ID)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *PaymentHandler) handlePaymentSuccess(paymentIntentID string) {
	now := time.Now()

	// Update payment status
	h.DB.Exec("UPDATE payments SET status = 'succeeded', updated_at = ? WHERE stripe_payment_intent_id = ?",
		now, paymentIntentID)

	// Check for lifetime membership
	var lmCount int
	h.DB.Get(&lmCount, "SELECT COUNT(*) FROM lifetime_memberships WHERE stripe_payment_intent_id = ?", paymentIntentID)
	if lmCount > 0 {
		var lm struct {
			UserID string
		}
		h.DB.Get(&lm, "SELECT user_id FROM lifetime_memberships WHERE stripe_payment_intent_id = ?", paymentIntentID)
		if lm.UserID != "" {
			h.activateLifetimeMembership(lm.UserID, paymentIntentID)
		}
		return
	}

	// Update booking payment status (existing flow)
	var payment models.Payment
	err := h.DB.Get(&payment, "SELECT * FROM payments WHERE stripe_payment_intent_id = ?", paymentIntentID)
	if err != nil {
		return
	}

	if payment.BookingID != nil {
		h.DB.Exec("UPDATE bookings SET payment_status = 'paid', status = 'confirmed', payout_status = 'ready' WHERE id = ?", *payment.BookingID)
	}
}

func (h *PaymentHandler) handlePaymentFailure(paymentIntentID string) {
	now := time.Now()
	h.DB.Exec("UPDATE payments SET status = 'failed', updated_at = ? WHERE stripe_payment_intent_id = ?",
		now, paymentIntentID)
}

func (h *PaymentHandler) getOrCreateCustomer(userID string) (string, error) {
	// Check if user already has a Stripe customer ID
	var stripeID *string
	err := h.DB.Get(&stripeID, "SELECT stripe_customer_id FROM users WHERE id = ?", userID)
	if err == nil && stripeID != nil && *stripeID != "" {
		return *stripeID, nil
	}

	// Get user details
	var user models.User
	err = h.DB.Get(&user, "SELECT id, wallet_address, display_name, email FROM users WHERE id = ?", userID)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}

	// Create Stripe customer
	params := &stripe.CustomerParams{
		Metadata: map[string]string{
			"user_id":        userID,
			"wallet_address": user.WalletAddress,
		},
	}
	if user.Email != nil {
		params.Email = user.Email
	}
	if user.DisplayName != nil {
		params.Name = user.DisplayName
	}

	cust, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe customer creation failed: %w", err)
	}

	// Save to DB
	h.DB.Exec("UPDATE users SET stripe_customer_id = ? WHERE id = ?", cust.ID, userID)

	return cust.ID, nil
}

func (h *PaymentHandler) getOrCreateConnectedAccount(userID, country string) (string, error) {
	var user struct {
		ID              string  `db:"id"`
		Email           *string `db:"email"`
		DisplayName     *string `db:"display_name"`
		StripeAccountID *string `db:"stripe_account_id"`
	}
	err := h.DB.Get(&user, "SELECT id, email, display_name, stripe_account_id FROM users WHERE id = ?", userID)
	if err != nil {
		return "", fmt.Errorf("user not found")
	}
	if user.StripeAccountID != nil && *user.StripeAccountID != "" {
		return *user.StripeAccountID, nil
	}

	params := &stripe.AccountParams{
		Type:    stripe.String("express"),
		Country: stripe.String(country),
		Email:   user.Email,
		Capabilities: &stripe.AccountCapabilitiesParams{
			CardPayments: &stripe.AccountCapabilitiesCardPaymentsParams{Requested: stripe.Bool(true)},
			Transfers:    &stripe.AccountCapabilitiesTransfersParams{Requested: stripe.Bool(true)},
		},
		BusinessProfile: &stripe.AccountBusinessProfileParams{
			ProductDescription: stripe.String("Travel marketplace services, local tours, and paid creator gigs"),
		},
		Metadata: map[string]string{"user_id": userID},
	}

	acct, err := account.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create payout account: %w", err)
	}

	_, _ = h.DB.Exec(`UPDATE users SET stripe_account_id = ?, stripe_charges_enabled = ?, stripe_payouts_enabled = ?, stripe_details_submitted = ? WHERE id = ?`,
		acct.ID, acct.ChargesEnabled, acct.PayoutsEnabled, acct.DetailsSubmitted, userID)
	return acct.ID, nil
}

// ===================== LIFETIME MEMBERSHIP ($100 one-time) =====================

// CreateLifetimePaymentIntent creates a Stripe PaymentIntent for the $100 lifetime membership.
// This is a one-time (non-recurring) payment.
// POST /payments/lifetime
func (h *PaymentHandler) CreateLifetimePaymentIntent(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Check if already has lifetime
	var tier *string
	h.DB.Get(&tier, "SELECT membership_tier FROM users WHERE id = ?", userID)
	if tier != nil && *tier == "lifetime" {
		writeError(w, http.StatusBadRequest, "You already have lifetime membership")
		return
	}

	amountCents := 10000 // $100.00 USD

	stripeCustomerID, err := h.getOrCreateCustomer(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create payment customer")
		return
	}

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(int64(amountCents)),
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		Customer: stripe.String(stripeCustomerID),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
		Metadata: map[string]string{
			"user_id":    userID,
			"type":       "lifetime_membership",
			"amount_usd": "100",
		},
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create payment intent")
		return
	}

	// Record pending lifetime purchase
	lmID := uuid.New().String()
	now := time.Now()
	_, _ = h.DB.Exec(`INSERT INTO lifetime_memberships (id, user_id, stripe_payment_intent_id, amount_cents, currency, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		lmID, userID, pi.ID, amountCents, "usd", "pending", now)

	writeJSON(w, http.StatusCreated, paymentResponse{
		PaymentID:    lmID,
		ClientSecret: pi.ClientSecret,
		AmountCents:  amountCents,
		Currency:     "usd",
	})
}

// GetMembershipStatus returns lifetime membership state + daily free explore remaining (0 or 1).
// GET /membership/status
func (h *PaymentHandler) GetMembershipStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var user models.User
	err := h.DB.Get(&user, `SELECT id, membership_tier, lifetime_paid_at, membership_amount_cents, 
		free_explore_last_date, free_explore_uses_today FROM users WHERE id = ?`, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	hasMembership := user.MembershipTier != nil && *user.MembershipTier == "lifetime"

	today := time.Now().Format("2006-01-02")
	usesToday := 0
	if user.FreeExploreLastDate != nil && *user.FreeExploreLastDate == today {
		usesToday = user.FreeExploreUsesToday
	}
	remaining := 1 - usesToday
	if remaining < 0 {
		remaining = 0
	}

	status := lifetimeStatus{
		HasMembership:        hasMembership,
		Tier:                 "",
		AmountCents:          user.MembershipAmountCents,
		FreeExploreUsed:      usesToday,
		FreeExploreRemaining: remaining,
	}
	if hasMembership && user.MembershipTier != nil {
		status.Tier = *user.MembershipTier
	}
	if user.LifetimePaidAt != nil {
		status.PaidAt = user.LifetimePaidAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, status)
}

// activateLifetimeMembership is called when a lifetime payment succeeds.
func (h *PaymentHandler) activateLifetimeMembership(userID string, paymentIntentID string) {
	now := time.Now()
	tier := "lifetime"

	h.DB.Exec(`UPDATE users SET 
		membership_tier = ?, 
		lifetime_paid_at = ?, 
		membership_amount_cents = 10000,
		updated_at = ?
		WHERE id = ?`, tier, now, now, userID)

	h.DB.Exec(`UPDATE lifetime_memberships SET status = 'succeeded' WHERE stripe_payment_intent_id = ?`, paymentIntentID)

	// Record in payments history too
	_, _ = h.DB.Exec(`INSERT INTO payments (id, user_id, stripe_payment_intent_id, amount_cents, currency, status, created_at, updated_at)
		VALUES (?, ?, ?, 10000, 'usd', 'succeeded', ?, ?)`,
		uuid.New().String(), userID, paymentIntentID, now, now)
}

// ===================== FREE EXPLORE QUOTA (1 per day) =====================

func (h *PaymentHandler) canUseFreeExplore(userID string) bool {
	today := time.Now().Format("2006-01-02")

	var lastDate *string
	var uses int
	err := h.DB.QueryRow(`SELECT free_explore_last_date, free_explore_uses_today FROM users WHERE id = ?`, userID).
		Scan(&lastDate, &uses)
	if err != nil {
		return true // new user gets the free use
	}

	if lastDate == nil || *lastDate != today {
		return true
	}
	return uses < 1
}

func (h *PaymentHandler) recordFreeExploreUse(userID string) {
	today := time.Now().Format("2006-01-02")

	// Reset if new day
	var lastDate *string
	h.DB.Get(&lastDate, "SELECT free_explore_last_date FROM users WHERE id = ?", userID)

	if lastDate == nil || *lastDate != today {
		h.DB.Exec(`UPDATE users SET free_explore_last_date = ?, free_explore_uses_today = 1 WHERE id = ?`, today, userID)
	} else {
		h.DB.Exec(`UPDATE users SET free_explore_uses_today = free_explore_uses_today + 1 WHERE id = ?`, userID)
	}
}

// ConsumeFreeExplore consumes the user's 1 free daily explore use (if available).
// Called by mobile when user views full content on their free use.
// POST /explore/consume-free
func (h *PaymentHandler) ConsumeFreeExplore(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	if !h.canUseFreeExplore(userID) {
		writeError(w, http.StatusForbidden, "No free uses remaining today")
		return
	}

	h.recordFreeExploreUse(userID)

	// Return updated status
	h.GetMembershipStatus(w, r)
}

// ConfirmLifetimeDev is a development helper that immediately activates lifetime membership.
// In production the Stripe webhook will do this after successful PaymentIntent.
// POST /payments/lifetime/confirm-dev
func (h *PaymentHandler) ConfirmLifetimeDev(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Idempotent
	var tier *string
	h.DB.Get(&tier, "SELECT membership_tier FROM users WHERE id = ?", userID)
	if tier != nil && *tier == "lifetime" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_active"})
		return
	}

	// Create a fake payment intent id for the record
	fakePI := "dev_lifetime_" + uuid.New().String()

	now := time.Now()
	h.DB.Exec(`INSERT INTO lifetime_memberships (id, user_id, stripe_payment_intent_id, amount_cents, currency, status, created_at)
		VALUES (?, ?, ?, 10000, 'usd', 'succeeded', ?)`,
		uuid.New().String(), userID, fakePI, now)

	h.activateLifetimeMembership(userID, fakePI)

	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}
