package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type ExperienceHandler struct {
	DB *sqlx.DB
}

type experienceRequest struct {
	Title        string   `json:"title"`
	Category     string   `json:"category"`
	Description  *string  `json:"description"`
	LocationName *string  `json:"location_name"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	PriceMon     *float64 `json:"price_mon"`
	PriceUSD     *float64 `json:"price_usd"`
	MaxGuests    *int     `json:"max_guests"`
	MinGuests    *int     `json:"min_guests"`
	ImageURL     *string  `json:"image_url"`
	IsActive     *bool    `json:"is_active"`
}

type userExperienceRequest struct {
	Title         string   `json:"title"`
	Category      string   `json:"category"`
	Description   string   `json:"description"`
	LocationName  string   `json:"locationName"`
	Latitude      float64  `json:"latitude"`
	Longitude     float64  `json:"longitude"`
	PriceUSD      float64  `json:"priceUsd"`
	MaxGuests     int      `json:"maxGuests"`
	MinGuests     *int     `json:"minGuests"`
	DurationMin   *int     `json:"durationMin"`
	MeetingPoint  *string  `json:"meetingPoint"`
	WhatToBring   *string  `json:"whatToBring"`
	Languages     *string  `json:"languages"`
	CoverPhotoURL *string  `json:"coverPhotoUrl"`
	GalleryURLs   *string  `json:"galleryUrls"`
	SaveAsDraft   bool     `json:"saveAsDraft"`
	Waypoints     []waypointInput `json:"waypoints"`
}

type waypointInput struct {
	Name              string   `json:"name"`
	Description       *string  `json:"description"`
	Latitude          float64  `json:"latitude"`
	Longitude         float64  `json:"longitude"`
	OrderIndex        int      `json:"orderIndex"`
	RadiusMeters      *float64 `json:"radiusMeters"`
	IsPhotoCheckpoint bool     `json:"isPhotoCheckpoint"`
	PhotoPrompt       *string  `json:"photoPrompt"`
	EstimatedDuration *int     `json:"estimatedDurationMin"`
}

// ListExperiences handles GET /experiences
func (h *ExperienceHandler) ListExperiences(w http.ResponseWriter, r *http.Request) {
	var experiences []models.Experience
	err := h.DB.Select(&experiences, "SELECT * FROM experiences WHERE is_active = 1 AND status = 'active' ORDER BY created_at DESC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch experiences")
		return
	}
	if experiences == nil {
		experiences = []models.Experience{}
	}
	writeJSON(w, http.StatusOK, experiences)
}

// GetExperience handles GET /experiences/{slug}
func (h *ExperienceHandler) GetExperience(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var exp models.Experience
	err := h.DB.Get(&exp, "SELECT * FROM experiences WHERE slug = ?", slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "Experience not found")
		return
	}
	writeJSON(w, http.StatusOK, exp)
}

// ── User-created experiences ─────────────────────────────────────────

// CreateUserExperience lets any authenticated user create their own experience.
// POST /experiences/create
func (h *ExperienceHandler) CreateUserExperience(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req userExperienceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Title == "" || req.Category == "" || req.Description == "" {
		writeError(w, http.StatusBadRequest, "title, category, and description are required")
		return
	}
	if req.LocationName == "" {
		writeError(w, http.StatusBadRequest, "locationName is required")
		return
	}
	if req.PriceUSD <= 0 {
		writeError(w, http.StatusBadRequest, "priceUsd must be greater than 0")
		return
	}
	if req.MaxGuests <= 0 {
		writeError(w, http.StatusBadRequest, "maxGuests must be at least 1")
		return
	}

	slug := generateSlug(req.Title) + "-" + uuid.New().String()[:8]
	now := time.Now()

	status := "active"
	if req.SaveAsDraft {
		status = "draft"
	}

	desc := req.Description
	locName := req.LocationName
	priceUSD := req.PriceUSD
	maxGuests := req.MaxGuests

	exp := models.Experience{
		ID:            uuid.New().String(),
		Slug:          &slug,
		Title:         req.Title,
		Category:      req.Category,
		Description:   &desc,
		LocationName:  &locName,
		Latitude:      &req.Latitude,
		Longitude:     &req.Longitude,
		PriceUSD:      &priceUSD,
		MaxGuests:     &maxGuests,
		MinGuests:     req.MinGuests,
		IsActive:      !req.SaveAsDraft,
		CreatorID:     &userID,
		Status:        status,
		DurationMin:   req.DurationMin,
		MeetingPoint:  req.MeetingPoint,
		WhatToBring:   req.WhatToBring,
		Languages:     req.Languages,
		CoverPhotoURL: req.CoverPhotoURL,
		GalleryURLs:   req.GalleryURLs,
		CreatedAt:     now,
		UpdatedAt:     &now,
	}

	tx, err := h.DB.Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to start transaction")
		return
	}

	_, err = tx.Exec(`INSERT INTO experiences
		(id, slug, title, category, description, location_name, latitude, longitude,
		 price_usd, max_guests, min_guests, is_active, creator_id, status,
		 duration_min, meeting_point, what_to_bring, languages,
		 cover_photo_url, gallery_urls, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		exp.ID, exp.Slug, exp.Title, exp.Category, exp.Description, exp.LocationName,
		exp.Latitude, exp.Longitude, exp.PriceUSD, exp.MaxGuests, exp.MinGuests,
		exp.IsActive, exp.CreatorID, exp.Status, exp.DurationMin, exp.MeetingPoint,
		exp.WhatToBring, exp.Languages, exp.CoverPhotoURL, exp.GalleryURLs,
		exp.CreatedAt, exp.UpdatedAt)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, "Failed to create experience")
		return
	}

	// Create waypoints if provided
	for _, wp := range req.Waypoints {
		radius := 50.0
		if wp.RadiusMeters != nil {
			radius = *wp.RadiusMeters
		}
		_, err = tx.Exec(`INSERT INTO waypoints
			(id, experience_id, name, description, latitude, longitude, order_index,
			 radius_meters, is_photo_checkpoint, photo_prompt, estimated_duration_min)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), exp.ID, wp.Name, wp.Description,
			wp.Latitude, wp.Longitude, wp.OrderIndex, radius,
			wp.IsPhotoCheckpoint, wp.PhotoPrompt, wp.EstimatedDuration)
		if err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "Failed to create waypoint")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save experience")
		return
	}

	writeJSON(w, http.StatusCreated, exp)
}

// UpdateUserExperience lets a creator edit their own experience.
// PUT /experiences/mine/{id}
func (h *ExperienceHandler) UpdateUserExperience(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	expID := chi.URLParam(r, "id")

	var existing models.Experience
	err := h.DB.Get(&existing, "SELECT * FROM experiences WHERE id = ? AND creator_id = ?", expID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Experience not found or you don't own it")
		return
	}

	var req userExperienceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now()

	if req.Title != "" {
		existing.Title = req.Title
		slug := generateSlug(req.Title) + "-" + uuid.New().String()[:8]
		existing.Slug = &slug
	}
	if req.Category != "" {
		existing.Category = req.Category
	}
	if req.Description != "" {
		existing.Description = &req.Description
	}
	if req.LocationName != "" {
		existing.LocationName = &req.LocationName
	}
	if req.Latitude != 0 {
		existing.Latitude = &req.Latitude
	}
	if req.Longitude != 0 {
		existing.Longitude = &req.Longitude
	}
	if req.PriceUSD > 0 {
		existing.PriceUSD = &req.PriceUSD
	}
	if req.MaxGuests > 0 {
		existing.MaxGuests = &req.MaxGuests
	}
	existing.MinGuests = req.MinGuests
	existing.DurationMin = req.DurationMin
	existing.MeetingPoint = req.MeetingPoint
	existing.WhatToBring = req.WhatToBring
	existing.Languages = req.Languages
	existing.CoverPhotoURL = req.CoverPhotoURL
	existing.GalleryURLs = req.GalleryURLs
	existing.UpdatedAt = &now

	if req.SaveAsDraft {
		existing.Status = "draft"
		existing.IsActive = false
	}

	_, err = h.DB.Exec(`UPDATE experiences SET
		slug=?, title=?, category=?, description=?, location_name=?, latitude=?, longitude=?,
		price_usd=?, max_guests=?, min_guests=?, is_active=?, status=?,
		duration_min=?, meeting_point=?, what_to_bring=?, languages=?,
		cover_photo_url=?, gallery_urls=?, updated_at=?
		WHERE id=? AND creator_id=?`,
		existing.Slug, existing.Title, existing.Category, existing.Description, existing.LocationName,
		existing.Latitude, existing.Longitude, existing.PriceUSD, existing.MaxGuests, existing.MinGuests,
		existing.IsActive, existing.Status, existing.DurationMin, existing.MeetingPoint,
		existing.WhatToBring, existing.Languages, existing.CoverPhotoURL, existing.GalleryURLs,
		existing.UpdatedAt, expID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update experience")
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

// ListMyExperiences returns all experiences created by the current user.
// GET /experiences/mine
func (h *ExperienceHandler) ListMyExperiences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var experiences []models.Experience
	err := h.DB.Select(&experiences, "SELECT * FROM experiences WHERE creator_id = ? ORDER BY created_at DESC", userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch experiences")
		return
	}
	if experiences == nil {
		experiences = []models.Experience{}
	}
	writeJSON(w, http.StatusOK, experiences)
}

// PublishExperience changes a draft to active.
// PATCH /experiences/mine/{id}/publish
func (h *ExperienceHandler) PublishExperience(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	expID := chi.URLParam(r, "id")

	var exp models.Experience
	err := h.DB.Get(&exp, "SELECT * FROM experiences WHERE id = ? AND creator_id = ?", expID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Experience not found or you don't own it")
		return
	}

	if exp.Status == "active" {
		writeError(w, http.StatusBadRequest, "Experience is already published")
		return
	}

	now := time.Now()
	_, err = h.DB.Exec("UPDATE experiences SET status = 'active', is_active = 1, updated_at = ? WHERE id = ? AND creator_id = ?",
		now, expID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to publish experience")
		return
	}

	exp.Status = "active"
	exp.IsActive = true
	exp.UpdatedAt = &now
	writeJSON(w, http.StatusOK, exp)
}

// UnpublishExperience takes an experience offline (back to draft).
// PATCH /experiences/mine/{id}/unpublish
func (h *ExperienceHandler) UnpublishExperience(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	expID := chi.URLParam(r, "id")

	now := time.Now()
	result, err := h.DB.Exec("UPDATE experiences SET status = 'draft', is_active = 0, updated_at = ? WHERE id = ? AND creator_id = ?",
		now, expID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to unpublish experience")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "Experience not found or you don't own it")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "draft"})
}

// DeleteUserExperience removes a user's experience (only if no active bookings).
// DELETE /experiences/mine/{id}
func (h *ExperienceHandler) DeleteUserExperience(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	expID := chi.URLParam(r, "id")

	// Check ownership
	var exp models.Experience
	err := h.DB.Get(&exp, "SELECT * FROM experiences WHERE id = ? AND creator_id = ?", expID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Experience not found or you don't own it")
		return
	}

	// Check no active bookings
	var activeBookings int
	h.DB.Get(&activeBookings, "SELECT COUNT(*) FROM bookings WHERE experience_id = ? AND status IN ('pending','confirmed')", expID)
	if activeBookings > 0 {
		writeError(w, http.StatusConflict, "Cannot delete: there are active bookings for this experience")
		return
	}

	// Delete waypoints first, then experience
	h.DB.Exec("DELETE FROM waypoints WHERE experience_id = ?", expID)
	h.DB.Exec("DELETE FROM experiences WHERE id = ? AND creator_id = ?", expID, userID)

	writeJSON(w, http.StatusOK, map[string]string{"deleted": expID})
}

// GetMyExperienceBookings returns bookings for a creator's experience.
// GET /experiences/mine/{id}/bookings
func (h *ExperienceHandler) GetMyExperienceBookings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	expID := chi.URLParam(r, "id")

	// Verify ownership
	var exp models.Experience
	err := h.DB.Get(&exp, "SELECT * FROM experiences WHERE id = ? AND creator_id = ?", expID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Experience not found or you don't own it")
		return
	}

	var bookings []models.Booking
	err = h.DB.Select(&bookings, "SELECT * FROM bookings WHERE experience_id = ? ORDER BY created_at DESC", expID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch bookings")
		return
	}
	if bookings == nil {
		bookings = []models.Booking{}
	}
	writeJSON(w, http.StatusOK, bookings)
}

// ── Admin-only routes (unchanged) ───────────────────────────────────

// CreateExperience handles POST /admin/experiences (admin-created)
func (h *ExperienceHandler) CreateExperience(w http.ResponseWriter, r *http.Request) {
	var req experienceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Title == "" || req.Category == "" {
		writeError(w, http.StatusBadRequest, "title and category are required")
		return
	}

	slug := generateSlug(req.Title)
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	exp := models.Experience{
		ID:           uuid.New().String(),
		Slug:         &slug,
		Title:        req.Title,
		Category:     req.Category,
		Description:  req.Description,
		LocationName: req.LocationName,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		PriceMon:     req.PriceMon,
		PriceUSD:     req.PriceUSD,
		MaxGuests:    req.MaxGuests,
		ImageURL:     req.ImageURL,
		IsActive:     isActive,
		Status:       "active",
		CreatedAt:    time.Now(),
	}

	_, err := h.DB.Exec(`INSERT INTO experiences
		(id, slug, title, category, description, location_name, latitude, longitude,
		 price_mon, price_usd, max_guests, image_url, is_active, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		exp.ID, exp.Slug, exp.Title, exp.Category, exp.Description, exp.LocationName,
		exp.Latitude, exp.Longitude, exp.PriceMon, exp.PriceUSD, exp.MaxGuests, exp.ImageURL,
		exp.IsActive, exp.Status, exp.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create experience")
		return
	}

	writeJSON(w, http.StatusCreated, exp)
}

// UpdateExperience handles PUT /admin/experiences/{id}
func (h *ExperienceHandler) UpdateExperience(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var existing models.Experience
	err := h.DB.Get(&existing, "SELECT * FROM experiences WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Experience not found")
		return
	}

	var req experienceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Title != "" {
		existing.Title = req.Title
		slug := generateSlug(req.Title)
		existing.Slug = &slug
	}
	if req.Category != "" {
		existing.Category = req.Category
	}
	if req.Description != nil {
		existing.Description = req.Description
	}
	if req.LocationName != nil {
		existing.LocationName = req.LocationName
	}
	if req.Latitude != nil {
		existing.Latitude = req.Latitude
	}
	if req.Longitude != nil {
		existing.Longitude = req.Longitude
	}
	if req.PriceMon != nil {
		existing.PriceMon = req.PriceMon
	}
	if req.PriceUSD != nil {
		existing.PriceUSD = req.PriceUSD
	}
	if req.MaxGuests != nil {
		existing.MaxGuests = req.MaxGuests
	}
	if req.ImageURL != nil {
		existing.ImageURL = req.ImageURL
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	_, err = h.DB.Exec(`UPDATE experiences SET slug=?, title=?, category=?, description=?, location_name=?, latitude=?, longitude=?, price_mon=?, price_usd=?, max_guests=?, image_url=?, is_active=? WHERE id=?`,
		existing.Slug, existing.Title, existing.Category, existing.Description, existing.LocationName,
		existing.Latitude, existing.Longitude, existing.PriceMon, existing.PriceUSD, existing.MaxGuests,
		existing.ImageURL, existing.IsActive, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update experience")
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

func generateSlug(title string) string {
	slug := strings.ToLower(title)
	slug = strings.ReplaceAll(slug, " ", "-")
	var result []byte
	for i := 0; i < len(slug); i++ {
		c := slug[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		}
	}
	return string(result)
}
