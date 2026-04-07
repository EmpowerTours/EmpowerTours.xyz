package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

func contains(jsonArray string, value string) bool {
	return strings.Contains(strings.ToLower(jsonArray), strings.ToLower(value))
}

type AdminHandler struct {
	DB *sqlx.DB
}

// ListApplications handles GET /admin/applications
// Query params: ?status=pending|docs_required|docs_submitted|interview_scheduled|approved|rejected
func (h *AdminHandler) ListApplications(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	var apps []models.Application
	var err error

	if status != "" {
		err = h.DB.Select(&apps, "SELECT * FROM applications WHERE status = ? ORDER BY created_at ASC", status)
	} else {
		// Default: show all non-approved/rejected (actionable items)
		err = h.DB.Select(&apps, "SELECT * FROM applications WHERE status NOT IN ('approved', 'rejected') ORDER BY created_at ASC")
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch applications")
		return
	}
	if apps == nil {
		apps = []models.Application{}
	}
	writeJSON(w, http.StatusOK, apps)
}

// RequestDocs handles POST /admin/applications/{id}/request-docs
// For workers: moves status to 'docs_required', applicant must upload INE, recommendation letter, job situation
func (h *AdminHandler) RequestDocs(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")

	var app models.Application
	if err := h.DB.Get(&app, "SELECT * FROM applications WHERE id = ?", appID); err != nil {
		writeError(w, http.StatusNotFound, "Application not found")
		return
	}

	if app.Role != "worker" {
		writeError(w, http.StatusBadRequest, "Document requests are only for worker applications")
		return
	}

	if app.Status != "pending" {
		writeError(w, http.StatusBadRequest, "Application must be in pending status")
		return
	}

	_, err := h.DB.Exec(
		"UPDATE applications SET status = 'docs_required', interview_status = 'docs_pending' WHERE id = ?",
		appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update application")
		return
	}

	app.Status = "docs_required"
	app.InterviewStatus = "docs_pending"
	writeJSON(w, http.StatusOK, app)
}

// ScheduleInterview handles POST /admin/applications/{id}/schedule-interview
// Admin sets interview date/location after docs are submitted and reviewed
func (h *AdminHandler) ScheduleInterview(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")

	var req struct {
		InterviewDate     string `json:"interviewDate"`     // RFC3339 format
		InterviewLocation string `json:"interviewLocation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.InterviewDate == "" || req.InterviewLocation == "" {
		writeError(w, http.StatusBadRequest, "interviewDate and interviewLocation are required")
		return
	}

	interviewTime, err := time.Parse(time.RFC3339, req.InterviewDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "interviewDate must be RFC3339 format (e.g. 2026-04-15T10:00:00-06:00)")
		return
	}

	var app models.Application
	if err := h.DB.Get(&app, "SELECT * FROM applications WHERE id = ?", appID); err != nil {
		writeError(w, http.StatusNotFound, "Application not found")
		return
	}

	if !app.DocumentsComplete {
		writeError(w, http.StatusBadRequest, "Applicant has not submitted all required documents")
		return
	}

	_, err = h.DB.Exec(
		`UPDATE applications SET status = 'interview_scheduled', interview_status = 'scheduled',
		 interview_date = ?, interview_location = ? WHERE id = ?`,
		interviewTime, req.InterviewLocation, appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to schedule interview")
		return
	}

	app.Status = "interview_scheduled"
	app.InterviewStatus = "scheduled"
	app.InterviewDate = &interviewTime
	app.InterviewLocation = &req.InterviewLocation
	writeJSON(w, http.StatusOK, app)
}

// CompleteInterview handles POST /admin/applications/{id}/complete-interview
// Admin marks interview as completed with notes, then can approve/reject
func (h *AdminHandler) CompleteInterview(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")

	var req struct {
		Notes  string `json:"notes"`
		Result string `json:"result"` // "pass" or "fail"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Result != "pass" && req.Result != "fail" {
		writeError(w, http.StatusBadRequest, "result must be 'pass' or 'fail'")
		return
	}

	var app models.Application
	if err := h.DB.Get(&app, "SELECT * FROM applications WHERE id = ?", appID); err != nil {
		writeError(w, http.StatusNotFound, "Application not found")
		return
	}

	if app.InterviewStatus != "scheduled" {
		writeError(w, http.StatusBadRequest, "No interview scheduled for this application")
		return
	}

	adminID := middleware.GetUserID(r.Context())
	now := time.Now()

	if req.Result == "fail" {
		_, err := h.DB.Exec(
			`UPDATE applications SET status = 'rejected', interview_status = 'completed',
			 interview_notes = ?, reviewed_by = ?, reviewed_at = ? WHERE id = ?`,
			req.Notes, adminID, now, appID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to update application")
			return
		}
		app.Status = "rejected"
	} else {
		// Pass — ready for final approval
		_, err := h.DB.Exec(
			`UPDATE applications SET interview_status = 'completed', interview_notes = ? WHERE id = ?`,
			req.Notes, appID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to update application")
			return
		}
	}

	app.InterviewStatus = "completed"
	app.InterviewNotes = &req.Notes
	writeJSON(w, http.StatusOK, app)
}

// ApproveApplication handles POST /admin/applications/{id}/approve
func (h *AdminHandler) ApproveApplication(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	adminID := middleware.GetUserID(r.Context())

	var app models.Application
	if err := h.DB.Get(&app, "SELECT * FROM applications WHERE id = ?", appID); err != nil {
		writeError(w, http.StatusNotFound, "Application not found")
		return
	}

	// Workers must complete interview before approval
	if app.Role == "worker" && app.InterviewStatus != "completed" {
		writeError(w, http.StatusBadRequest, "Worker must complete interview before approval")
		return
	}

	// Customers can be approved from pending
	if app.Status != "pending" && app.InterviewStatus != "completed" {
		writeError(w, http.StatusBadRequest, "Application cannot be approved in current state")
		return
	}

	now := time.Now()

	tx, err := h.DB.Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE applications SET status = 'approved', reviewed_by = ?, reviewed_at = ? WHERE id = ?",
		adminID, now, appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update application")
		return
	}

	if app.UserID != nil {
		isDriver := false
		if app.Role == "worker" && app.Skills != nil {
			if contains(*app.Skills, "driving") {
				isDriver = true
			}
		}

		_, err = tx.Exec("UPDATE users SET membership_tier = 'standard', role = ?, is_driver = ?, updated_at = ? WHERE id = ?",
			app.Role, isDriver, now, *app.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to update user")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to commit transaction")
		return
	}

	app.Status = "approved"
	app.ReviewedBy = &adminID
	app.ReviewedAt = &now
	writeJSON(w, http.StatusOK, app)
}

// RejectApplication handles POST /admin/applications/{id}/reject
func (h *AdminHandler) RejectApplication(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	adminID := middleware.GetUserID(r.Context())

	var app models.Application
	if err := h.DB.Get(&app, "SELECT * FROM applications WHERE id = ?", appID); err != nil {
		writeError(w, http.StatusNotFound, "Application not found")
		return
	}

	if app.Status == "approved" || app.Status == "rejected" {
		writeError(w, http.StatusBadRequest, "Application already finalized")
		return
	}

	now := time.Now()
	_, err := h.DB.Exec("UPDATE applications SET status = 'rejected', reviewed_by = ?, reviewed_at = ? WHERE id = ?",
		adminID, now, appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update application")
		return
	}

	app.Status = "rejected"
	app.ReviewedBy = &adminID
	app.ReviewedAt = &now
	writeJSON(w, http.StatusOK, app)
}
