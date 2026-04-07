package models

import "time"

type Ride struct {
	ID               string     `json:"id" db:"id"`
	RiderID          *string    `json:"riderId" db:"rider_id"`
	DriverID         *string    `json:"driverId" db:"driver_id"`
	PickupLat        *float64   `json:"pickupLat" db:"pickup_lat"`
	PickupLng        *float64   `json:"pickupLng" db:"pickup_lng"`
	PickupAddress    *string    `json:"pickupAddress" db:"pickup_address"`
	DropoffLat       *float64   `json:"dropoffLat" db:"dropoff_lat"`
	DropoffLng       *float64   `json:"dropoffLng" db:"dropoff_lng"`
	DropoffAddress   *string    `json:"dropoffAddress" db:"dropoff_address"`
	Status           string     `json:"status" db:"status"`
	EstimatedFareMon *float64   `json:"estimatedFareMon" db:"estimated_fare_mon"`
	ActualFareMon    *float64   `json:"actualFareMon" db:"actual_fare_mon"`
	PaymentTxHash    *string    `json:"paymentTxHash" db:"payment_tx_hash"`
	CreatedAt        time.Time  `json:"createdAt" db:"created_at"`
	CompletedAt      *time.Time `json:"completedAt" db:"completed_at"`
}
