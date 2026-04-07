package models

import "time"

type ExperienceSession struct {
	ID                  string     `json:"id" db:"id"`
	BookingID           string     `json:"bookingId" db:"booking_id"`
	UserID              string     `json:"userId" db:"user_id"`
	GuideID             *string    `json:"guideId" db:"guide_id"`
	ExperienceID        string     `json:"experienceId" db:"experience_id"`
	Status              string     `json:"status" db:"status"`
	StartedAt           *time.Time `json:"startedAt" db:"started_at"`
	CompletedAt         *time.Time `json:"completedAt" db:"completed_at"`
	TotalDistanceMeters float64    `json:"totalDistanceMeters" db:"total_distance_meters"`
	AvgSpeedKmh         float64    `json:"avgSpeedKmh" db:"avg_speed_kmh"`
	CreatedAt           time.Time  `json:"createdAt" db:"created_at"`
}

type LocationPoint struct {
	ID         string    `json:"id" db:"id"`
	SessionID  string    `json:"sessionId" db:"session_id"`
	UserID     string    `json:"userId" db:"user_id"`
	Latitude   float64   `json:"latitude" db:"latitude"`
	Longitude  float64   `json:"longitude" db:"longitude"`
	Altitude   *float64  `json:"altitude" db:"altitude"`
	Accuracy   *float64  `json:"accuracy" db:"accuracy"`
	Speed      *float64  `json:"speed" db:"speed"`
	Heading    *float64  `json:"heading" db:"heading"`
	RecordedAt time.Time `json:"recordedAt" db:"recorded_at"`
}

type CheckpointPhoto struct {
	ID         string    `json:"id" db:"id"`
	SessionID  string    `json:"sessionId" db:"session_id"`
	WaypointID string    `json:"waypointId" db:"waypoint_id"`
	UserID     string    `json:"userId" db:"user_id"`
	PhotoURL   string    `json:"photoUrl" db:"photo_url"`
	Latitude   float64   `json:"latitude" db:"latitude"`
	Longitude  float64   `json:"longitude" db:"longitude"`
	Caption    *string   `json:"caption" db:"caption"`
	TakenAt    time.Time `json:"takenAt" db:"taken_at"`
}
