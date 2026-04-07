package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type ApplicationHandler struct {
	DB *sqlx.DB
}

var validRoles = map[string]bool{"customer": true, "worker": true, "creator": true}

type applyRequest struct {
	Role           string  `json:"role"` // "customer", "worker", "creator"
	FullName       string  `json:"fullName"`
	Email          string  `json:"email"`
	Phone          *string `json:"phone"`
	Location       *string `json:"location"`
	Reason         string  `json:"reason"`
	ReferralCode   *string `json:"referralCode"`
	ContentConsent bool    `json:"contentConsent"`

	// Worker fields
	Skills       *string `json:"skills"`       // JSON array: ["driving","farming","landscaping","masonry"]
	VehicleType  *string `json:"vehicleType"`  // car, van, suv, atv
	Availability *string `json:"availability"` // fulltime, parttime, weekends

	// Customer fields
	Interests *string `json:"interests"` // JSON array: ["ranch","climbing","dining","transport"]
	GroupSize *int    `json:"groupSize"`
}

// SubmitApplication handles POST /apply
func (h *ApplicationHandler) SubmitApplication(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req applyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.FullName == "" || req.Email == "" || req.Reason == "" {
		writeError(w, http.StatusBadRequest, "fullName, email, and reason are required")
		return
	}

	if !validRoles[req.Role] {
		writeError(w, http.StatusBadRequest, "role must be 'customer', 'worker', or 'creator'")
		return
	}

	// Everyone must consent to being recorded/used for brand content
	if !req.ContentConsent {
		writeError(w, http.StatusBadRequest, "You must consent to being recorded and used for EmpowerTours brand content")
		return
	}

	// Creator role → redirect to Encanta
	if req.Role == "creator" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"redirect": true,
			"url":      "https://encanta.app",
			"message":  "Content creators apply through Encanta. You'll complete the audition process there.",
		})
		return
	}

	// Worker-specific validation
	if req.Role == "worker" && req.Skills == nil {
		writeError(w, http.StatusBadRequest, "Workers must specify at least one skill")
		return
	}

	// Check for existing pending/approved application
	var count int
	err := h.DB.Get(&count, "SELECT COUNT(*) FROM applications WHERE user_id = ? AND status IN ('pending', 'approved')", userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "You already have a pending or approved application")
		return
	}

	app := models.Application{
		ID:             uuid.New().String(),
		UserID:         &userID,
		Role:           req.Role,
		FullName:       req.FullName,
		Email:          req.Email,
		Phone:          req.Phone,
		Location:       req.Location,
		Reason:         req.Reason,
		ReferralCode:   req.ReferralCode,
		ContentConsent: true,
		Skills:         req.Skills,
		VehicleType:    req.VehicleType,
		Availability:   req.Availability,
		Interests:      req.Interests,
		GroupSize:       req.GroupSize,
		Status:         "pending",
		CreatedAt:      time.Now(),
	}

	_, err = h.DB.Exec(`INSERT INTO applications
		(id, user_id, role, full_name, email, phone, location, reason, referral_code,
		 content_consent, skills, vehicle_type, availability, interests, group_size, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		app.ID, app.UserID, app.Role, app.FullName, app.Email, app.Phone, app.Location,
		app.Reason, app.ReferralCode, app.ContentConsent, app.Skills, app.VehicleType,
		app.Availability, app.Interests, app.GroupSize, app.Status, app.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create application")
		return
	}

	writeJSON(w, http.StatusCreated, app)
}

// GetApplicationStatus handles GET /apply/status
func (h *ApplicationHandler) GetApplicationStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var app models.Application
	err := h.DB.Get(&app, "SELECT * FROM applications WHERE user_id = ? ORDER BY created_at DESC LIMIT 1", userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "No application found")
		return
	}

	writeJSON(w, http.StatusOK, app)
}

// SubmitDocuments handles POST /apply/documents
// Workers upload required docs: INE, recommendation letter, current job situation
func (h *ApplicationHandler) SubmitDocuments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var app models.Application
	err := h.DB.Get(&app, "SELECT * FROM applications WHERE user_id = ? ORDER BY created_at DESC LIMIT 1", userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "No application found")
		return
	}

	if app.Status != "docs_required" {
		writeError(w, http.StatusBadRequest, "Documents are not currently required for this application")
		return
	}

	var req struct {
		INEDocURL               string `json:"ineDocUrl"`
		RecommendationLetterURL string `json:"recommendationLetterUrl"`
		CurrentJobSituation     string `json:"currentJobSituation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.INEDocURL == "" || req.RecommendationLetterURL == "" || req.CurrentJobSituation == "" {
		writeError(w, http.StatusBadRequest, "ineDocUrl, recommendationLetterUrl, and currentJobSituation are all required")
		return
	}

	_, err = h.DB.Exec(
		`UPDATE applications SET ine_doc_url = ?, recommendation_letter_url = ?,
		 current_job_situation = ?, documents_complete = 1, interview_status = 'docs_submitted'
		 WHERE id = ?`,
		req.INEDocURL, req.RecommendationLetterURL, req.CurrentJobSituation, app.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save documents")
		return
	}

	app.INEDocURL = &req.INEDocURL
	app.RecommendationLetterURL = &req.RecommendationLetterURL
	app.CurrentJobSituation = &req.CurrentJobSituation
	app.DocumentsComplete = true
	app.InterviewStatus = "docs_submitted"
	writeJSON(w, http.StatusOK, app)
}
