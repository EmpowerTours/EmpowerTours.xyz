package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type WaypointHandler struct {
	DB *sqlx.DB
}

type createWaypointRequest struct {
	ExperienceID         string   `json:"experienceId"`
	Name                 string   `json:"name"`
	Description          *string  `json:"description"`
	Latitude             float64  `json:"latitude"`
	Longitude            float64  `json:"longitude"`
	OrderIndex           int      `json:"orderIndex"`
	RadiusMeters         *float64 `json:"radiusMeters"`
	IsPhotoCheckpoint    bool     `json:"isPhotoCheckpoint"`
	PhotoPrompt          *string  `json:"photoPrompt"`
	EstimatedDurationMin *int     `json:"estimatedDurationMin"`
}

// ListWaypoints returns all waypoints for an experience.
// GET /experiences/{slug}/waypoints
func (h *WaypointHandler) ListWaypoints(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	// Get experience by slug
	var exp models.Experience
	err := h.DB.Get(&exp, "SELECT * FROM experiences WHERE slug = ?", slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "Experience not found")
		return
	}

	var waypoints []models.Waypoint
	err = h.DB.Select(&waypoints, "SELECT * FROM waypoints WHERE experience_id = ? ORDER BY order_index ASC", exp.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch waypoints")
		return
	}
	if waypoints == nil {
		waypoints = []models.Waypoint{}
	}
	writeJSON(w, http.StatusOK, waypoints)
}

// CreateWaypoint adds a waypoint to an experience (admin only).
// POST /admin/waypoints
func (h *WaypointHandler) CreateWaypoint(w http.ResponseWriter, r *http.Request) {
	var req createWaypointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ExperienceID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "experienceId and name are required")
		return
	}

	radius := 50.0
	if req.RadiusMeters != nil {
		radius = *req.RadiusMeters
	}

	wp := models.Waypoint{
		ID:                   uuid.New().String(),
		ExperienceID:         req.ExperienceID,
		Name:                 req.Name,
		Description:          req.Description,
		Latitude:             req.Latitude,
		Longitude:            req.Longitude,
		OrderIndex:           req.OrderIndex,
		RadiusMeters:         radius,
		IsPhotoCheckpoint:    req.IsPhotoCheckpoint,
		PhotoPrompt:          req.PhotoPrompt,
		EstimatedDurationMin: req.EstimatedDurationMin,
	}

	_, err := h.DB.Exec(`INSERT INTO waypoints
		(id, experience_id, name, description, latitude, longitude, order_index, radius_meters, is_photo_checkpoint, photo_prompt, estimated_duration_min)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wp.ID, wp.ExperienceID, wp.Name, wp.Description, wp.Latitude, wp.Longitude,
		wp.OrderIndex, wp.RadiusMeters, wp.IsPhotoCheckpoint, wp.PhotoPrompt, wp.EstimatedDurationMin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create waypoint")
		return
	}

	writeJSON(w, http.StatusCreated, wp)
}

// UpdateWaypoint modifies a waypoint (admin only).
// PUT /admin/waypoints/{id}
func (h *WaypointHandler) UpdateWaypoint(w http.ResponseWriter, r *http.Request) {
	wpID := chi.URLParam(r, "id")

	var req createWaypointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	_, err := h.DB.Exec(`UPDATE waypoints SET
		name = ?, description = ?, latitude = ?, longitude = ?, order_index = ?,
		radius_meters = ?, is_photo_checkpoint = ?, photo_prompt = ?, estimated_duration_min = ?
		WHERE id = ?`,
		req.Name, req.Description, req.Latitude, req.Longitude, req.OrderIndex,
		req.RadiusMeters, req.IsPhotoCheckpoint, req.PhotoPrompt, req.EstimatedDurationMin, wpID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update waypoint")
		return
	}

	var wp models.Waypoint
	h.DB.Get(&wp, "SELECT * FROM waypoints WHERE id = ?", wpID)
	writeJSON(w, http.StatusOK, wp)
}

// DeleteWaypoint removes a waypoint (admin only).
// DELETE /admin/waypoints/{id}
func (h *WaypointHandler) DeleteWaypoint(w http.ResponseWriter, r *http.Request) {
	wpID := chi.URLParam(r, "id")

	result, err := h.DB.Exec("DELETE FROM waypoints WHERE id = ?", wpID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete waypoint")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "Waypoint not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"deleted": wpID})
}
