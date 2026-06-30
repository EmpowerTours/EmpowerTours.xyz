package models

import "time"

type User struct {
	ID                string    `json:"id" db:"id"`
	WalletAddress     string    `json:"walletAddress" db:"wallet_address"`
	DisplayName       *string   `json:"displayName" db:"display_name"`
	Email             *string   `json:"email" db:"email"`
	Phone             *string   `json:"phone" db:"phone"`
	MembershipTier    *string   `json:"membershipTier" db:"membership_tier"`
	MembershipTokenID *int64    `json:"membershipTokenId" db:"membership_token_id"`
	Nonce             *string   `json:"nonce" db:"nonce"`
	IsAdmin           bool      `json:"isAdmin" db:"is_admin"`
	IsDriver          bool      `json:"isDriver" db:"is_driver"`
	CreatedAt         time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time `json:"updatedAt" db:"updated_at"`

	// Lifetime membership
	LifetimePaidAt        *time.Time `json:"lifetimePaidAt" db:"lifetime_paid_at"`
	MembershipAmountCents int        `json:"membershipAmountCents" db:"membership_amount_cents"`

	// Free daily explore quota (1 free use/day)
	FreeExploreLastDate  *string `json:"freeExploreLastDate" db:"free_explore_last_date"`
	FreeExploreUsesToday int     `json:"freeExploreUsesToday" db:"free_explore_uses_today"`

	StripeCustomerID       *string `json:"stripeCustomerId" db:"stripe_customer_id"`
	StripeAccountID        *string `json:"stripeAccountId" db:"stripe_account_id"`
	StripeChargesEnabled   bool    `json:"stripeChargesEnabled" db:"stripe_charges_enabled"`
	StripePayoutsEnabled   bool    `json:"stripePayoutsEnabled" db:"stripe_payouts_enabled"`
	StripeDetailsSubmitted bool    `json:"stripeDetailsSubmitted" db:"stripe_details_submitted"`
}

type Application struct {
	ID             string  `json:"id" db:"id"`
	UserID         *string `json:"userId" db:"user_id"`
	Role           string  `json:"role" db:"role"` // "customer", "worker", "creator"
	FullName       string  `json:"fullName" db:"full_name"`
	Email          string  `json:"email" db:"email"`
	Phone          *string `json:"phone" db:"phone"`
	Location       *string `json:"location" db:"location"`
	Reason         string  `json:"reason" db:"reason"`
	ReferralCode   *string `json:"referralCode" db:"referral_code"`
	ContentConsent bool    `json:"contentConsent" db:"content_consent"`

	// Worker fields
	Skills       *string `json:"skills" db:"skills"`
	VehicleType  *string `json:"vehicleType" db:"vehicle_type"`
	Availability *string `json:"availability" db:"availability"`

	// Worker documents
	INEDocURL               *string `json:"ineDocUrl" db:"ine_doc_url"`
	RecommendationLetterURL *string `json:"recommendationLetterUrl" db:"recommendation_letter_url"`
	CurrentJobSituation     *string `json:"currentJobSituation" db:"current_job_situation"`
	DocumentsComplete       bool    `json:"documentsComplete" db:"documents_complete"`

	// Interview
	InterviewStatus   string     `json:"interviewStatus" db:"interview_status"` // none, docs_pending, docs_submitted, scheduled, completed, no_show
	InterviewDate     *time.Time `json:"interviewDate" db:"interview_date"`
	InterviewLocation *string    `json:"interviewLocation" db:"interview_location"`
	InterviewNotes    *string    `json:"interviewNotes" db:"interview_notes"`

	// Customer fields
	Interests *string `json:"interests" db:"interests"`
	GroupSize *int    `json:"groupSize" db:"group_size"`

	Status     string     `json:"status" db:"status"` // pending, docs_required, interview_scheduled, approved, rejected
	ReviewedBy *string    `json:"reviewedBy" db:"reviewed_by"`
	ReviewedAt *time.Time `json:"reviewedAt" db:"reviewed_at"`
	TxHash     *string    `json:"txHash" db:"tx_hash"`
	CreatedAt  time.Time  `json:"createdAt" db:"created_at"`
}
