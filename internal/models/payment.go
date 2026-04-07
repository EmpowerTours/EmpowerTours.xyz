package models

import "time"

type Payment struct {
	ID                    string    `json:"id" db:"id"`
	UserID                string    `json:"userId" db:"user_id"`
	BookingID             *string   `json:"bookingId" db:"booking_id"`
	StripePaymentIntentID *string   `json:"stripePaymentIntentId" db:"stripe_payment_intent_id"`
	StripeCustomerID      *string   `json:"stripeCustomerId" db:"stripe_customer_id"`
	AmountCents           int       `json:"amountCents" db:"amount_cents"`
	Currency              string    `json:"currency" db:"currency"`
	Status                string    `json:"status" db:"status"`
	Description           *string   `json:"description" db:"description"`
	ReceiptURL            *string   `json:"receiptUrl" db:"receipt_url"`
	CreatedAt             time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt             time.Time `json:"updatedAt" db:"updated_at"`
}
