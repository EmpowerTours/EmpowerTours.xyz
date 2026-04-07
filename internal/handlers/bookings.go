package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type BookingHandler struct {
	DB *sqlx.DB
}

type bookingRequest struct {
	ExperienceID string `json:"experience_id"`
	BookingDate  string `json:"booking_date"`
	GuestCount   int    `json:"guest_count"`
}

// CreateBooking handles POST /bookings
func (h *BookingHandler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req bookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ExperienceID == "" || req.BookingDate == "" || req.GuestCount < 1 {
		writeError(w, http.StatusBadRequest, "experience_id, booking_date, and guest_count (>0) are required")
		return
	}

	// Verify experience exists and is active
	var exp models.Experience
	err := h.DB.Get(&exp, "SELECT * FROM experiences WHERE id = ? AND is_active = 1", req.ExperienceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Experience not found or inactive")
		return
	}

	// Check max guests
	if exp.MaxGuests != nil && req.GuestCount > *exp.MaxGuests {
		writeError(w, http.StatusBadRequest, "Guest count exceeds maximum allowed")
		return
	}

	// Calculate total
	var totalMon *float64
	if exp.PriceMon != nil {
		t := *exp.PriceMon * float64(req.GuestCount)
		totalMon = &t
	}

	booking := models.Booking{
		ID:            uuid.New().String(),
		UserID:        &userID,
		ExperienceID:  &req.ExperienceID,
		BookingDate:   &req.BookingDate,
		GuestCount:    &req.GuestCount,
		Status:        "pending",
		PaymentStatus: "unpaid",
		TotalMon:      totalMon,
		CreatedAt:     time.Now(),
	}

	_, err = h.DB.Exec(`INSERT INTO bookings (id, user_id, experience_id, booking_date, guest_count, status, payment_status, total_mon, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		booking.ID, booking.UserID, booking.ExperienceID, booking.BookingDate, booking.GuestCount,
		booking.Status, booking.PaymentStatus, booking.TotalMon, booking.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create booking")
		return
	}

	writeJSON(w, http.StatusCreated, booking)
}

// ListBookings handles GET /bookings
func (h *BookingHandler) ListBookings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var bookings []models.Booking
	err := h.DB.Select(&bookings, "SELECT * FROM bookings WHERE user_id = ? ORDER BY created_at DESC", userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch bookings")
		return
	}
	if bookings == nil {
		bookings = []models.Booking{}
	}
	writeJSON(w, http.StatusOK, bookings)
}

// GetBooking handles GET /bookings/{id}
func (h *BookingHandler) GetBooking(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	bookingID := chi.URLParam(r, "id")

	var booking models.Booking
	err := h.DB.Get(&booking, "SELECT * FROM bookings WHERE id = ? AND user_id = ?", bookingID, userID)
	if err != nil {
		// Allow admin to view any booking
		if middleware.IsAdmin(r.Context()) {
			err = h.DB.Get(&booking, "SELECT * FROM bookings WHERE id = ?", bookingID)
		}
		if err != nil {
			writeError(w, http.StatusNotFound, "Booking not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, booking)
}

// CancelBooking handles PATCH /bookings/{id}/cancel
func (h *BookingHandler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	bookingID := chi.URLParam(r, "id")

	var booking models.Booking
	err := h.DB.Get(&booking, "SELECT * FROM bookings WHERE id = ? AND user_id = ?", bookingID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Booking not found")
		return
	}

	if booking.Status == "cancelled" {
		writeError(w, http.StatusBadRequest, "Booking is already cancelled")
		return
	}

	if booking.Status == "completed" {
		writeError(w, http.StatusBadRequest, "Cannot cancel a completed booking")
		return
	}

	_, err = h.DB.Exec("UPDATE bookings SET status = 'cancelled' WHERE id = ?", bookingID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to cancel booking")
		return
	}

	booking.Status = "cancelled"
	writeJSON(w, http.StatusOK, booking)
}

// ConfirmBooking handles PATCH /bookings/{id}/confirm (admin only)
func (h *BookingHandler) ConfirmBooking(w http.ResponseWriter, r *http.Request) {
	bookingID := chi.URLParam(r, "id")

	var booking models.Booking
	err := h.DB.Get(&booking, "SELECT * FROM bookings WHERE id = ?", bookingID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Booking not found")
		return
	}

	if booking.Status != "pending" {
		writeError(w, http.StatusBadRequest, "Only pending bookings can be confirmed")
		return
	}

	_, err = h.DB.Exec("UPDATE bookings SET status = 'confirmed' WHERE id = ?", bookingID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to confirm booking")
		return
	}

	booking.Status = "confirmed"
	writeJSON(w, http.StatusOK, booking)
}
