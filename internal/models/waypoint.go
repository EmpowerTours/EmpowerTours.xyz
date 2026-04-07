package models

import "time"

type Waypoint struct {
	ID                   string    `json:"id" db:"id"`
	ExperienceID         string    `json:"experienceId" db:"experience_id"`
	Name                 string    `json:"name" db:"name"`
	Description          *string   `json:"description" db:"description"`
	Latitude             float64   `json:"latitude" db:"latitude"`
	Longitude            float64   `json:"longitude" db:"longitude"`
	OrderIndex           int       `json:"orderIndex" db:"order_index"`
	RadiusMeters         float64   `json:"radiusMeters" db:"radius_meters"`
	IsPhotoCheckpoint    bool      `json:"isPhotoCheckpoint" db:"is_photo_checkpoint"`
	PhotoPrompt          *string   `json:"photoPrompt" db:"photo_prompt"`
	EstimatedDurationMin *int      `json:"estimatedDurationMin" db:"estimated_duration_min"`
	CreatedAt            time.Time `json:"createdAt" db:"created_at"`
}
