package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/empowertours/empowertours-app/internal/realtime"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type TrackingHandler struct {
	DB  *sqlx.DB
	Hub *realtime.Hub
}

type locationUpdate struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Altitude  *float64 `json:"altitude"`
	Accuracy  *float64 `json:"accuracy"`
	Speed     *float64 `json:"speed"`
	Heading   *float64 `json:"heading"`
}

type locationBatch struct {
	Points []locationUpdate `json:"points"`
}

// RecordLocation stores a GPS point and broadcasts it to session viewers.
// POST /sessions/{id}/location
func (h *TrackingHandler) RecordLocation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	sessionID := chi.URLParam(r, "id")

	// Verify active session
	var session models.ExperienceSession
	err := h.DB.Get(&session, "SELECT * FROM experience_sessions WHERE id = ? AND status = 'active'", sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Active session not found")
		return
	}

	if session.UserID != userID && (session.GuideID == nil || *session.GuideID != userID) {
		writeError(w, http.StatusForbidden, "Not authorized to update this session")
		return
	}

	var req locationUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now()
	point := models.LocationPoint{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		UserID:     userID,
		Latitude:   req.Latitude,
		Longitude:  req.Longitude,
		Altitude:   req.Altitude,
		Accuracy:   req.Accuracy,
		Speed:      req.Speed,
		Heading:    req.Heading,
		RecordedAt: now,
	}

	_, err = h.DB.Exec(`INSERT INTO location_history
		(id, session_id, user_id, latitude, longitude, altitude, accuracy, speed, heading, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		point.ID, point.SessionID, point.UserID, point.Latitude, point.Longitude,
		point.Altitude, point.Accuracy, point.Speed, point.Heading, point.RecordedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to record location")
		return
	}

	// Broadcast to all session viewers
	h.Hub.Broadcast(&realtime.WSMessage{
		Type:      realtime.MsgTypeLocation,
		SessionID: sessionID,
		UserID:    userID,
		Payload: realtime.LocationPayload{
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Altitude:  req.Altitude,
			Accuracy:  req.Accuracy,
			Speed:     req.Speed,
			Heading:   req.Heading,
		},
		Timestamp: now,
	})

	writeJSON(w, http.StatusOK, point)
}

// RecordLocationBatch stores multiple GPS points at once (for offline sync).
// POST /sessions/{id}/location/batch
func (h *TrackingHandler) RecordLocationBatch(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	sessionID := chi.URLParam(r, "id")

	var session models.ExperienceSession
	err := h.DB.Get(&session, "SELECT * FROM experience_sessions WHERE id = ? AND (status = 'active' OR status = 'paused')", sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}

	if session.UserID != userID && (session.GuideID == nil || *session.GuideID != userID) {
		writeError(w, http.StatusForbidden, "Not authorized")
		return
	}

	var req locationBatch
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.Points) == 0 {
		writeError(w, http.StatusBadRequest, "No points provided")
		return
	}
	if len(req.Points) > 500 {
		writeError(w, http.StatusBadRequest, "Maximum 500 points per batch")
		return
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to start transaction")
		return
	}

	now := time.Now()
	for _, p := range req.Points {
		_, err = tx.Exec(`INSERT INTO location_history
			(id, session_id, user_id, latitude, longitude, altitude, accuracy, speed, heading, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), sessionID, userID,
			p.Latitude, p.Longitude, p.Altitude, p.Accuracy, p.Speed, p.Heading, now)
		if err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "Failed to record locations")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to commit locations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"recorded": len(req.Points)})
}

// GetLocationHistory returns all GPS points for a session.
// GET /sessions/{id}/location
func (h *TrackingHandler) GetLocationHistory(w http.ResponseWriter, r *http.Request) {
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

	var points []models.LocationPoint
	err = h.DB.Select(&points, "SELECT * FROM location_history WHERE session_id = ? ORDER BY recorded_at ASC", sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch location history")
		return
	}
	if points == nil {
		points = []models.LocationPoint{}
	}
	writeJSON(w, http.StatusOK, points)
}
