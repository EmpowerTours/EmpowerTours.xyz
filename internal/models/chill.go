package models

import "time"

type UserProfile struct {
	ID              string   `json:"id" db:"id"`
	DisplayName     *string  `json:"displayName" db:"display_name"`
	Bio             *string  `json:"bio" db:"bio"`
	ProfilePhotoURL *string  `json:"profilePhotoUrl" db:"profile_photo_url"`
	Interests       *string  `json:"interests" db:"interests"`
	CurrentLat      *float64 `json:"currentLat" db:"current_lat"`
	CurrentLng      *float64 `json:"currentLng" db:"current_lng"`
	CurrentCity     *string  `json:"currentCity" db:"current_city"`
	Age             *int     `json:"age" db:"age"`
	IsDiscoverable  bool     `json:"isDiscoverable" db:"is_discoverable"`
}

type ChillRequest struct {
	ID         string    `json:"id" db:"id"`
	FromUserID string    `json:"fromUserId" db:"from_user_id"`
	ToUserID   string    `json:"toUserId" db:"to_user_id"`
	Status     string    `json:"status" db:"status"`
	Message    *string   `json:"message" db:"message"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
}

type ChillMatch struct {
	ID               string     `json:"id" db:"id"`
	User1ID          string     `json:"user1Id" db:"user1_id"`
	User2ID          string     `json:"user2Id" db:"user2_id"`
	Status           string     `json:"status" db:"status"`
	MeetLat          *float64   `json:"meetLat" db:"meet_lat"`
	MeetLng          *float64   `json:"meetLng" db:"meet_lng"`
	MeetLocationName *string    `json:"meetLocationName" db:"meet_location_name"`
	MeetDate         *time.Time `json:"meetDate" db:"meet_date"`
	MeetExperienceID *string    `json:"meetExperienceId" db:"meet_experience_id"`
	LastMessage      *string    `json:"lastMessage" db:"last_message"`
	LastMessageAt    *time.Time `json:"lastMessageAt" db:"last_message_at"`
	CreatedAt        time.Time  `json:"createdAt" db:"created_at"`
}

// ChillMatchWithProfile includes the other user's profile info
type ChillMatchWithProfile struct {
	ChillMatch
	OtherUser UserProfile `json:"otherUser"`
}

type TravelPlan struct {
	ID             string    `json:"id" db:"id"`
	UserID         string    `json:"userId" db:"user_id"`
	Destination    string    `json:"destination" db:"destination"`
	DestinationLat *float64  `json:"destinationLat" db:"destination_lat"`
	DestinationLng *float64  `json:"destinationLng" db:"destination_lng"`
	Country        *string   `json:"country" db:"country"`
	ArriveDate     string    `json:"arriveDate" db:"arrive_date"`
	DepartDate     string    `json:"departDate" db:"depart_date"`
	Notes          *string   `json:"notes" db:"notes"`
	IsActive       bool      `json:"isActive" db:"is_active"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
}

// TravelMatchProfile is a user profile enriched with their travel plan
type TravelMatchProfile struct {
	UserProfile
	TravelPlan TravelPlan `json:"travelPlan"`
}
