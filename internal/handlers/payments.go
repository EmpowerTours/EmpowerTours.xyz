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
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/webhook"
)

type PaymentHandler struct {
	DB                *sqlx.DB
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
	amountCents := int(*exp.PriceUSD * 100) * guestCount

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
		},
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
	h.DB.Exec("UPDATE bookings SET stripe_payment_id = ?, total_usd_cents = ? WHERE id = ?",
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

	// Update booking payment status
	var payment models.Payment
	err := h.DB.Get(&payment, "SELECT * FROM payments WHERE stripe_payment_intent_id = ?", paymentIntentID)
	if err != nil {
		return
	}

	if payment.BookingID != nil {
		h.DB.Exec("UPDATE bookings SET payment_status = 'paid', status = 'confirmed' WHERE id = ?", *payment.BookingID)
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
	err = h.DB.Get(&user, "SELECT * FROM users WHERE id = ?", userID)
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
