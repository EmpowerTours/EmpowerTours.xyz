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

type RideHandler struct {
	DB *sqlx.DB
}

type rideRequest struct {
	PickupLat        *float64 `json:"pickup_lat"`
	PickupLng        *float64 `json:"pickup_lng"`
	PickupAddress    *string  `json:"pickup_address"`
	DropoffLat       *float64 `json:"dropoff_lat"`
	DropoffLng       *float64 `json:"dropoff_lng"`
	DropoffAddress   *string  `json:"dropoff_address"`
	EstimatedFareMon *float64 `json:"estimated_fare_mon"`
}

type statusRequest struct {
	Status string `json:"status"`
}

// RequestRide handles POST /rides/request
func (h *RideHandler) RequestRide(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req rideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.PickupLat == nil || req.PickupLng == nil || req.DropoffLat == nil || req.DropoffLng == nil {
		writeError(w, http.StatusBadRequest, "pickup and dropoff coordinates are required")
		return
	}

	ride := models.Ride{
		ID:               uuid.New().String(),
		RiderID:          &userID,
		PickupLat:        req.PickupLat,
		PickupLng:        req.PickupLng,
		PickupAddress:    req.PickupAddress,
		DropoffLat:       req.DropoffLat,
		DropoffLng:       req.DropoffLng,
		DropoffAddress:   req.DropoffAddress,
		Status:           "requested",
		EstimatedFareMon: req.EstimatedFareMon,
		CreatedAt:        time.Now(),
	}

	_, err := h.DB.Exec(`INSERT INTO rides (id, rider_id, pickup_lat, pickup_lng, pickup_address, dropoff_lat, dropoff_lng, dropoff_address, status, estimated_fare_mon, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ride.ID, ride.RiderID, ride.PickupLat, ride.PickupLng, ride.PickupAddress,
		ride.DropoffLat, ride.DropoffLng, ride.DropoffAddress, ride.Status, ride.EstimatedFareMon, ride.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create ride request")
		return
	}

	writeJSON(w, http.StatusCreated, ride)
}

// ListAvailableRides handles GET /rides/available
func (h *RideHandler) ListAvailableRides(w http.ResponseWriter, r *http.Request) {
	var rides []models.Ride
	err := h.DB.Select(&rides, "SELECT * FROM rides WHERE status = 'requested' AND driver_id IS NULL ORDER BY created_at ASC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch rides")
		return
	}
	if rides == nil {
		rides = []models.Ride{}
	}
	writeJSON(w, http.StatusOK, rides)
}

// AcceptRide handles POST /rides/{id}/accept
func (h *RideHandler) AcceptRide(w http.ResponseWriter, r *http.Request) {
	driverID := middleware.GetUserID(r.Context())
	rideID := chi.URLParam(r, "id")

	var ride models.Ride
	err := h.DB.Get(&ride, "SELECT * FROM rides WHERE id = ?", rideID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Ride not found")
		return
	}

	if ride.Status != "requested" {
		writeError(w, http.StatusBadRequest, "Ride is no longer available")
		return
	}

	if ride.RiderID != nil && *ride.RiderID == driverID {
		writeError(w, http.StatusBadRequest, "Cannot accept your own ride request")
		return
	}

	_, err = h.DB.Exec("UPDATE rides SET driver_id = ?, status = 'accepted' WHERE id = ? AND status = 'requested'", driverID, rideID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to accept ride")
		return
	}

	ride.DriverID = &driverID
	ride.Status = "accepted"
	writeJSON(w, http.StatusOK, ride)
}

// UpdateRideStatus handles PATCH /rides/{id}/status
func (h *RideHandler) UpdateRideStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	rideID := chi.URLParam(r, "id")

	var req statusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	validStatuses := map[string]bool{"in_progress": true, "completed": true}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "Status must be 'in_progress' or 'completed'")
		return
	}

	var ride models.Ride
	err := h.DB.Get(&ride, "SELECT * FROM rides WHERE id = ?", rideID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Ride not found")
		return
	}

	// Only driver or admin can update status
	isAdmin := middleware.IsAdmin(r.Context())
	if ride.DriverID == nil || (*ride.DriverID != userID && !isAdmin) {
		writeError(w, http.StatusForbidden, "Only the assigned driver can update ride status")
		return
	}

	if req.Status == "completed" {
		now := time.Now()
		_, err = h.DB.Exec("UPDATE rides SET status = ?, completed_at = ? WHERE id = ?", req.Status, now, rideID)
		ride.CompletedAt = &now
	} else {
		_, err = h.DB.Exec("UPDATE rides SET status = ? WHERE id = ?", req.Status, rideID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update ride status")
		return
	}

	ride.Status = req.Status
	writeJSON(w, http.StatusOK, ride)
}

// ListMyRides handles GET /rides/my
func (h *RideHandler) ListMyRides(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var rides []models.Ride
	err := h.DB.Select(&rides, "SELECT * FROM rides WHERE rider_id = ? OR driver_id = ? ORDER BY created_at DESC", userID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch rides")
		return
	}
	if rides == nil {
		rides = []models.Ride{}
	}
	writeJSON(w, http.StatusOK, rides)
}
