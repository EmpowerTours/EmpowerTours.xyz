package models

import "time"

type Booking struct {
	ID             string    `json:"id" db:"id"`
	UserID         *string   `json:"userId" db:"user_id"`
	ExperienceID   *string   `json:"experienceId" db:"experience_id"`
	BookingDate    *string   `json:"bookingDate" db:"booking_date"`
	GuestCount     *int      `json:"guestCount" db:"guest_count"`
	Status         string    `json:"status" db:"status"`
	PaymentTxHash  *string   `json:"paymentTxHash" db:"payment_tx_hash"`
	PaymentStatus  string    `json:"paymentStatus" db:"payment_status"`
	TotalMon       *float64  `json:"totalMon" db:"total_mon"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
}
