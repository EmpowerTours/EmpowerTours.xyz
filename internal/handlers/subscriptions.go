package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type SubscriptionHandler struct {
	DB *sqlx.DB
}

type startTrialRequest struct {
	Platform string `json:"platform"` // "apple" or "google"
}

type verifyReceiptRequest struct {
	Platform      string `json:"platform"`
	Receipt       string `json:"receipt"`
	TransactionID string `json:"transactionId"`
	ProductID     string `json:"productId"`
}

// GetSubscriptionStatus returns the current user's subscription state.
// GET /subscription/status
func (h *SubscriptionHandler) GetSubscriptionStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	status := h.getUserSubscriptionStatus(userID)
	writeJSON(w, http.StatusOK, status)
}

// StartTrial begins the 3-day free trial.
// POST /subscription/trial
func (h *SubscriptionHandler) StartTrial(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Check if user already had a trial
	var existingCount int
	h.DB.Get(&existingCount, "SELECT COUNT(*) FROM subscriptions WHERE user_id = ?", userID)
	if existingCount > 0 {
		writeError(w, http.StatusConflict, "You've already used your free trial")
		return
	}

	var req startTrialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Platform = "apple" // default
	}

	now := time.Now()
	trialEnd := now.Add(time.Duration(models.TrialDays) * 24 * time.Hour)

	sub := models.Subscription{
		ID:         uuid.New().String(),
		UserID:     userID,
		Platform:   req.Platform,
		ProductID:  models.SubscriptionProductID,
		Status:     "trialing",
		TrialStart: &now,
		TrialEnd:   &trialEnd,
		AutoRenew:  false,
		PriceLocal: intPtr(models.PriceMXN),
		Currency:   "MXN",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	_, err := h.DB.Exec(`INSERT INTO subscriptions
		(id, user_id, platform, product_id, status, trial_start, trial_end, auto_renew, price_local, currency, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.UserID, sub.Platform, sub.ProductID, sub.Status,
		sub.TrialStart, sub.TrialEnd, sub.AutoRenew, sub.PriceLocal, sub.Currency,
		sub.CreatedAt, sub.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to start trial")
		return
	}

	// Update user record
	h.DB.Exec("UPDATE users SET subscription_status = 'trialing', trial_ends_at = ? WHERE id = ?", trialEnd, userID)

	writeJSON(w, http.StatusCreated, h.getUserSubscriptionStatus(userID))
}

// VerifyReceipt validates an Apple/Google purchase receipt and activates subscription.
// POST /subscription/verify
func (h *SubscriptionHandler) VerifyReceipt(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req verifyReceiptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Receipt == "" || req.TransactionID == "" {
		writeError(w, http.StatusBadRequest, "receipt and transactionId are required")
		return
	}

	// In production, verify receipt with Apple/Google servers:
	// Apple: POST to https://buy.itunes.apple.com/verifyReceipt
	// Google: Use Google Play Developer API
	//
	// For now, we trust the client and activate.
	// TODO: Add server-side receipt validation before going live.

	now := time.Now()
	periodEnd := now.Add(365 * 24 * time.Hour) // 1 year

	// Check for existing subscription
	var existing models.Subscription
	err := h.DB.Get(&existing, "SELECT * FROM subscriptions WHERE user_id = ? ORDER BY created_at DESC LIMIT 1", userID)

	if err == nil {
		// Update existing
		h.DB.Exec(`UPDATE subscriptions SET
			status = 'active', original_transaction_id = ?, latest_receipt = ?,
			current_period_start = ?, current_period_end = ?, auto_renew = 1, updated_at = ?
			WHERE id = ?`,
			req.TransactionID, req.Receipt, now, periodEnd, now, existing.ID)
	} else {
		// Create new
		sub := models.Subscription{
			ID:                    uuid.New().String(),
			UserID:                userID,
			Platform:              req.Platform,
			ProductID:             models.SubscriptionProductID,
			OriginalTransactionID: &req.TransactionID,
			LatestReceipt:         &req.Receipt,
			Status:                "active",
			CurrentPeriodStart:    &now,
			CurrentPeriodEnd:      &periodEnd,
			AutoRenew:             true,
			PriceLocal:            intPtr(models.PriceMXN),
			Currency:              "MXN",
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		h.DB.Exec(`INSERT INTO subscriptions
			(id, user_id, platform, product_id, original_transaction_id, latest_receipt,
			 status, current_period_start, current_period_end, auto_renew, price_local, currency, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sub.ID, sub.UserID, sub.Platform, sub.ProductID, sub.OriginalTransactionID, sub.LatestReceipt,
			sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd, sub.AutoRenew,
			sub.PriceLocal, sub.Currency, sub.CreatedAt, sub.UpdatedAt)
	}

	// Update user record
	h.DB.Exec("UPDATE users SET subscription_status = 'active', subscription_ends_at = ? WHERE id = ?", periodEnd, userID)

	writeJSON(w, http.StatusOK, h.getUserSubscriptionStatus(userID))
}

// RestorePurchase re-checks subscription status from stored receipts.
// POST /subscription/restore
func (h *SubscriptionHandler) RestorePurchase(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var sub models.Subscription
	err := h.DB.Get(&sub, "SELECT * FROM subscriptions WHERE user_id = ? ORDER BY created_at DESC LIMIT 1", userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "No subscription found to restore")
		return
	}

	// Re-check if subscription is still valid
	now := time.Now()
	if sub.CurrentPeriodEnd != nil && sub.CurrentPeriodEnd.After(now) {
		h.DB.Exec("UPDATE users SET subscription_status = 'active', subscription_ends_at = ? WHERE id = ?",
			sub.CurrentPeriodEnd, userID)
		sub.Status = "active"
		h.DB.Exec("UPDATE subscriptions SET status = 'active', updated_at = ? WHERE id = ?", now, sub.ID)
	}

	writeJSON(w, http.StatusOK, h.getUserSubscriptionStatus(userID))
}

func (h *SubscriptionHandler) getUserSubscriptionStatus(userID string) models.SubscriptionStatus {
	now := time.Now()

	var sub models.Subscription
	err := h.DB.Get(&sub, "SELECT * FROM subscriptions WHERE user_id = ? ORDER BY created_at DESC LIMIT 1", userID)

	if err != nil {
		// No subscription at all
		return models.SubscriptionStatus{
			Status:       "none",
			CanAccessApp: false,
			ProductID:    models.SubscriptionProductID,
			Price:        "$1,500 MXN/year",
		}
	}

	status := models.SubscriptionStatus{
		ProductID: models.SubscriptionProductID,
		Price:     "$1,500 MXN/year",
	}

	switch sub.Status {
	case "trialing":
		if sub.TrialEnd != nil && sub.TrialEnd.After(now) {
			status.Status = "trialing"
			status.TrialEndsAt = sub.TrialEnd
			status.DaysRemaining = int(math.Ceil(sub.TrialEnd.Sub(now).Hours() / 24))
			status.CanAccessApp = true
		} else {
			// Trial expired
			status.Status = "expired"
			status.CanAccessApp = false
			h.DB.Exec("UPDATE subscriptions SET status = 'expired', updated_at = ? WHERE id = ?", now, sub.ID)
			h.DB.Exec("UPDATE users SET subscription_status = 'expired' WHERE id = ?", userID)
		}
	case "active":
		if sub.CurrentPeriodEnd != nil && sub.CurrentPeriodEnd.After(now) {
			status.Status = "active"
			status.SubscriptionEnds = sub.CurrentPeriodEnd
			status.DaysRemaining = int(math.Ceil(sub.CurrentPeriodEnd.Sub(now).Hours() / 24))
			status.CanAccessApp = true
		} else {
			status.Status = "expired"
			status.CanAccessApp = false
			h.DB.Exec("UPDATE subscriptions SET status = 'expired', updated_at = ? WHERE id = ?", now, sub.ID)
			h.DB.Exec("UPDATE users SET subscription_status = 'expired' WHERE id = ?", userID)
		}
	default:
		status.Status = sub.Status
		status.CanAccessApp = false
	}

	return status
}

func intPtr(v int) *int { return &v }
