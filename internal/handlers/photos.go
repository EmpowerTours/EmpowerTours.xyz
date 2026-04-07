package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/empowertours/empowertours-app/internal/realtime"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type PhotoHandler struct {
	DB  *sqlx.DB
	Hub *realtime.Hub
}

type uploadPhotoRequest struct {
	WaypointID string  `json:"waypointId"`
	PhotoURL   string  `json:"photoUrl"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Caption    *string `json:"caption"`
}

// UploadCheckpointPhoto records a photo taken at a waypoint during a session.
// POST /sessions/{id}/photos
func (h *PhotoHandler) UploadCheckpointPhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	sessionID := chi.URLParam(r, "id")

	var session models.ExperienceSession
	err := h.DB.Get(&session, "SELECT * FROM experience_sessions WHERE id = ? AND status = 'active'", sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Active session not found")
		return
	}

	if session.UserID != userID && (session.GuideID == nil || *session.GuideID != userID) {
		writeError(w, http.StatusForbidden, "Not authorized")
		return
	}

	var req uploadPhotoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.WaypointID == "" || req.PhotoURL == "" {
		writeError(w, http.StatusBadRequest, "waypointId and photoUrl are required")
		return
	}

	// Verify waypoint exists and belongs to the experience
	var wp models.Waypoint
	err = h.DB.Get(&wp, "SELECT * FROM waypoints WHERE id = ? AND experience_id = ?", req.WaypointID, session.ExperienceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Waypoint not found for this experience")
		return
	}

	// Check user is within waypoint radius
	dist := haversineDistance(req.Latitude, req.Longitude, wp.Latitude, wp.Longitude)
	if dist > wp.RadiusMeters {
		writeError(w, http.StatusBadRequest, "You are not close enough to this checkpoint")
		return
	}

	now := time.Now()
	photo := models.CheckpointPhoto{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		WaypointID: req.WaypointID,
		UserID:     userID,
		PhotoURL:   req.PhotoURL,
		Latitude:   req.Latitude,
		Longitude:  req.Longitude,
		Caption:    req.Caption,
		TakenAt:    now,
	}

	_, err = h.DB.Exec(`INSERT INTO checkpoint_photos
		(id, session_id, waypoint_id, user_id, photo_url, latitude, longitude, caption, taken_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		photo.ID, photo.SessionID, photo.WaypointID, photo.UserID,
		photo.PhotoURL, photo.Latitude, photo.Longitude, photo.Caption, photo.TakenAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save photo")
		return
	}

	// Broadcast photo event
	h.Hub.Broadcast(&realtime.WSMessage{
		Type:      realtime.MsgTypePhoto,
		SessionID: sessionID,
		UserID:    userID,
		Payload:   photo,
		Timestamp: now,
	})

	writeJSON(w, http.StatusCreated, photo)
}

// ListSessionPhotos returns all photos for a session.
// GET /sessions/{id}/photos
func (h *PhotoHandler) ListSessionPhotos(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	sessionID := chi.URLParam(r, "id")

	var session models.ExperienceSession
	err := h.DB.Get(&session, "SELECT * FROM experience_sessions WHERE id = ?", sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}

	if session.UserID != userID && (session.GuideID == nil || *session.GuideID != userID) {
		if !middleware.IsAdmin(r.Context()) {
			writeError(w, http.StatusForbidden, "Not authorized")
			return
		}
	}

	var photos []models.CheckpointPhoto
	err = h.DB.Select(&photos, "SELECT * FROM checkpoint_photos WHERE session_id = ? ORDER BY taken_at ASC", sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch photos")
		return
	}
	if photos == nil {
		photos = []models.CheckpointPhoto{}
	}
	writeJSON(w, http.StatusOK, photos)
}

// haversineDistance calculates distance in meters between two GPS coordinates.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000 // meters
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}
