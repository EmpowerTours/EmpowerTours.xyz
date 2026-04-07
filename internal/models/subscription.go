package models

import "time"

type Subscription struct {
	ID                    string     `json:"id" db:"id"`
	UserID                string     `json:"userId" db:"user_id"`
	Platform              string     `json:"platform" db:"platform"`
	ProductID             string     `json:"productId" db:"product_id"`
	OriginalTransactionID *string    `json:"originalTransactionId" db:"original_transaction_id"`
	LatestReceipt         *string    `json:"-" db:"latest_receipt"` // never expose to client
	Status                string     `json:"status" db:"status"`
	TrialStart            *time.Time `json:"trialStart" db:"trial_start"`
	TrialEnd              *time.Time `json:"trialEnd" db:"trial_end"`
	CurrentPeriodStart    *time.Time `json:"currentPeriodStart" db:"current_period_start"`
	CurrentPeriodEnd      *time.Time `json:"currentPeriodEnd" db:"current_period_end"`
	AutoRenew             bool       `json:"autoRenew" db:"auto_renew"`
	PriceLocal            *int       `json:"priceLocal" db:"price_local"`
	Currency              string     `json:"currency" db:"currency"`
	CreatedAt             time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt             time.Time  `json:"updatedAt" db:"updated_at"`
}

// SubscriptionStatus is the user-facing subscription state
type SubscriptionStatus struct {
	Status           string     `json:"status"` // none, trialing, active, expired
	TrialEndsAt      *time.Time `json:"trialEndsAt"`
	SubscriptionEnds *time.Time `json:"subscriptionEndsAt"`
	DaysRemaining    int        `json:"daysRemaining"`
	CanAccessApp     bool       `json:"canAccessApp"`
	ProductID        string     `json:"productId"`
	Price            string     `json:"price"` // "1,500 MXN/year"
}

const (
	SubscriptionProductID = "empowertours_annual"
	TrialDays             = 3
	PriceMXN              = 150000 // 1,500.00 MXN in centavos
)
