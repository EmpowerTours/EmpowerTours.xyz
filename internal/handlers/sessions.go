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

type SessionHandler struct {
	DB  *sqlx.DB
	Hub *realtime.Hub
}

type startSessionRequest struct {
	BookingID string  `json:"bookingId"`
	GuideID   *string `json:"guideId"`
}

// StartSession creates a new experience session from a confirmed booking.
// POST /sessions/start
func (h *SessionHandler) StartSession(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req startSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.BookingID == "" {
		writeError(w, http.StatusBadRequest, "bookingId is required")
		return
	}

	// Verify booking exists, belongs to user, and is confirmed
	var booking models.Booking
	err := h.DB.Get(&booking, "SELECT * FROM bookings WHERE id = ? AND user_id = ?", req.BookingID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Booking not found")
		return
	}
	if booking.Status != "confirmed" {
		writeError(w, http.StatusBadRequest, "Booking must be confirmed to start a session")
		return
	}

	// Check no active session for this booking
	var count int
	h.DB.Get(&count, "SELECT COUNT(*) FROM experience_sessions WHERE booking_id = ? AND status IN ('pending','active','paused')", req.BookingID)
	if count > 0 {
		writeError(w, http.StatusConflict, "An active session already exists for this booking")
		return
	}

	now := time.Now()
	session := models.ExperienceSession{
		ID:           uuid.New().String(),
		BookingID:    req.BookingID,
		UserID:       userID,
		GuideID:      req.GuideID,
		ExperienceID: *booking.ExperienceID,
		Status:       "active",
		StartedAt:    &now,
		CreatedAt:    now,
	}

	_, err = h.DB.Exec(`INSERT INTO experience_sessions
		(id, booking_id, user_id, guide_id, experience_id, status, started_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.BookingID, session.UserID, session.GuideID,
		session.ExperienceID, session.Status, session.StartedAt, session.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	// Broadcast session start
	h.Hub.Broadcast(&realtime.WSMessage{
		Type:      realtime.MsgTypeSessionStart,
		SessionID: session.ID,
		UserID:    userID,
		Payload:   session,
		Timestamp: now,
	})

	writeJSON(w, http.StatusCreated, session)
}

// EndSession completes or cancels a session.
// PATCH /sessions/{id}/end
func (h *SessionHandler) EndSession(w http.ResponseWriter, r *http.Request) {
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

	if session.Status != "active" && session.Status != "paused" {
		writeError(w, http.StatusBadRequest, "Session is not active")
		return
	}

	now := time.Now()
	_, err = h.DB.Exec("UPDATE experience_sessions SET status = 'completed', completed_at = ? WHERE id = ?", now, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to end session")
		return
	}

	session.Status = "completed"
	session.CompletedAt = &now

	h.Hub.Broadcast(&realtime.WSMessage{
		Type:      realtime.MsgTypeSessionEnd,
		SessionID: sessionID,
		UserID:    userID,
		Payload:   session,
		Timestamp: now,
	})

	writeJSON(w, http.StatusOK, session)
}

// GetSession returns session details including waypoints reached.
// GET /sessions/{id}
func (h *SessionHandler) GetSession(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, session)
}

// ListMySessions returns all sessions for the current user.
// GET /sessions/my
func (h *SessionHandler) ListMySessions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var sessions []models.ExperienceSession
	err := h.DB.Select(&sessions,
		"SELECT * FROM experience_sessions WHERE user_id = ? OR guide_id = ? ORDER BY created_at DESC",
		userID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch sessions")
		return
	}
	if sessions == nil {
		sessions = []models.ExperienceSession{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

// WebSocket handler for joining a live session
// GET /sessions/{id}/live
func (h *SessionHandler) LiveSession(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	sessionID := chi.URLParam(r, "id")

	// Verify session exists and user has access
	var session models.ExperienceSession
	err := h.DB.Get(&session, "SELECT * FROM experience_sessions WHERE id = ?", sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}

	if session.Status != "active" && session.Status != "paused" {
		writeError(w, http.StatusBadRequest, "Session is not active")
		return
	}

	realtime.ServeWS(h.Hub, userID, sessionID, w, r)
}
