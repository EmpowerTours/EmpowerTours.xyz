package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type ChillHandler struct {
	DB *sqlx.DB
}

type updateProfileRequest struct {
	Bio             *string  `json:"bio"`
	ProfilePhotoURL *string  `json:"profilePhotoUrl"`
	Interests       *string  `json:"interests"`
	CurrentLat      *float64 `json:"currentLat"`
	CurrentLng      *float64 `json:"currentLng"`
	CurrentCity     *string  `json:"currentCity"`
	Age             *int     `json:"age"`
	IsDiscoverable  *bool    `json:"isDiscoverable"`

	// Intent
	EmpowerRole      *string `json:"empowerRole"`
	BarterOk         *bool   `json:"barterOk"`
	PaidOk           *bool   `json:"paidOk"`
	PreferredRegions *string `json:"preferredRegions"`
	Skills           *string `json:"skills"`

	// Presence
	CurrentSpot     *string  `json:"currentSpot"`
	CurrentSpotLat  *float64 `json:"currentSpotLat"`
	CurrentSpotLng  *float64 `json:"currentSpotLng"`
	PlanningSpot    *string  `json:"planningSpot"`
	PlanningDate    *string  `json:"planningDate"`
	PlanningSpotLat *float64 `json:"planningSpotLat"`
	PlanningSpotLng *float64 `json:"planningSpotLng"`
}

type chillRequestBody struct {
	ToUserID string  `json:"toUserId"`
	Message  *string `json:"message"`
}

type scheduleMeetRequest struct {
	MeetDate *string `json:"meetDate"` // ISO 8601
}

// ── Profile ──────────────────────────────────────────────────────────

// UpdateProfile sets the user's bio, interests, location for Chill discovery.
// PUT /chill/profile
func (h *ChillHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	_, err := h.DB.Exec(`UPDATE users SET
		bio = COALESCE(?, bio),
		profile_photo_url = COALESCE(?, profile_photo_url),
		interests = COALESCE(?, interests),
		current_lat = COALESCE(?, current_lat),
		current_lng = COALESCE(?, current_lng),
		current_city = COALESCE(?, current_city),
		age = COALESCE(?, age),
		is_discoverable = COALESCE(?, is_discoverable),
		empower_role = COALESCE(?, empower_role),
		barter_ok = COALESCE(?, barter_ok),
		paid_ok = COALESCE(?, paid_ok),
		preferred_regions = COALESCE(?, preferred_regions),
		skills = COALESCE(?, skills),
		current_spot = COALESCE(?, current_spot),
		current_spot_lat = COALESCE(?, current_spot_lat),
		current_spot_lng = COALESCE(?, current_spot_lng),
		planning_spot = COALESCE(?, planning_spot),
		planning_date = COALESCE(?, planning_date),
		planning_spot_lat = COALESCE(?, planning_spot_lat),
		planning_spot_lng = COALESCE(?, planning_spot_lng),
		updated_at = ?
		WHERE id = ?`,
		req.Bio, req.ProfilePhotoURL, req.Interests,
		req.CurrentLat, req.CurrentLng, req.CurrentCity,
		req.Age, req.IsDiscoverable,
		req.EmpowerRole, req.BarterOk, req.PaidOk, req.PreferredRegions, req.Skills,
		req.CurrentSpot, req.CurrentSpotLat, req.CurrentSpotLng,
		req.PlanningSpot, req.PlanningDate, req.PlanningSpotLat, req.PlanningSpotLng,
		time.Now(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update profile")
		return
	}

	var profile models.UserProfile
	h.DB.Get(&profile, `SELECT id, display_name, bio, profile_photo_url, interests, current_lat, current_lng, current_city, age, is_discoverable,
		empower_role, barter_ok, paid_ok, preferred_regions, skills,
		current_spot, current_spot_lat, current_spot_lng,
		planning_spot, planning_date, planning_spot_lat, planning_spot_lng
		FROM users WHERE id = ?`, userID)
	writeJSON(w, http.StatusOK, profile)
}

// GetProfile returns the user's own profile.
// GET /chill/profile
func (h *ChillHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var profile models.UserProfile
	err := h.DB.Get(&profile, `SELECT id, display_name, bio, profile_photo_url, interests, current_lat, current_lng, current_city, age, is_discoverable,
		empower_role, barter_ok, paid_ok, preferred_regions, skills,
		current_spot, current_spot_lat, current_spot_lng,
		planning_spot, planning_date, planning_spot_lat, planning_spot_lng
		FROM users WHERE id = ?`, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Profile not found")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

// ── Discover (Browse Profiles) ───────────────────────────────────────

// Discover returns a list of users to browse for Chill.
// Excludes: self, already sent/received requests, existing matches.
// GET /chill/discover
func (h *ChillHandler) Discover(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var profiles []models.UserProfile
	err := h.DB.Select(&profiles, `
		SELECT id, display_name, bio, profile_photo_url, interests, current_lat, current_lng, current_city, age, is_discoverable,
			empower_role, barter_ok, paid_ok, preferred_regions, skills,
			current_spot, current_spot_lat, current_spot_lng,
			planning_spot, planning_date, planning_spot_lat, planning_spot_lng
		FROM users
		WHERE id != ?
		AND is_discoverable = 1
		AND bio IS NOT NULL
		AND bio != ''
		AND id NOT IN (SELECT to_user_id FROM chill_requests WHERE from_user_id = ?)
		AND id NOT IN (SELECT from_user_id FROM chill_requests WHERE to_user_id = ? AND status = 'declined')
		AND id NOT IN (SELECT user1_id FROM chill_matches WHERE user2_id = ?)
		AND id NOT IN (SELECT user2_id FROM chill_matches WHERE user1_id = ?)
		ORDER BY RANDOM()
		LIMIT 20`,
		userID, userID, userID, userID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load profiles")
		return
	}
	if profiles == nil {
		profiles = []models.UserProfile{}
	}
	writeJSON(w, http.StatusOK, profiles)
}

// ── Send Chill Request ───────────────────────────────────────────────

// SendChillRequest sends a "chill" to another user.
// If the other user already sent one to you, it auto-matches.
// POST /chill/request
func (h *ChillHandler) SendChillRequest(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req chillRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ToUserID == "" || req.ToUserID == userID {
		writeError(w, http.StatusBadRequest, "Invalid user")
		return
	}

	// Check if the other person already sent us a chill request (mutual match!)
	var existingReverse models.ChillRequest
	err := h.DB.Get(&existingReverse,
		"SELECT * FROM chill_requests WHERE from_user_id = ? AND to_user_id = ? AND status = 'pending'",
		req.ToUserID, userID)

	if err == nil {
		// MUTUAL MATCH! Create a match and calculate midpoint
		h.createMatch(w, userID, req.ToUserID, existingReverse.ID)
		return
	}

	// Check not already sent
	var count int
	h.DB.Get(&count, "SELECT COUNT(*) FROM chill_requests WHERE from_user_id = ? AND to_user_id = ?", userID, req.ToUserID)
	if count > 0 {
		writeError(w, http.StatusConflict, "Already sent a chill request to this user")
		return
	}

	// Create request
	cr := models.ChillRequest{
		ID:         uuid.New().String(),
		FromUserID: userID,
		ToUserID:   req.ToUserID,
		Status:     "pending",
		Message:    req.Message,
		CreatedAt:  time.Now(),
	}

	_, err = h.DB.Exec("INSERT INTO chill_requests (id, from_user_id, to_user_id, status, message, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		cr.ID, cr.FromUserID, cr.ToUserID, cr.Status, cr.Message, cr.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to send chill request")
		return
	}

	writeJSON(w, http.StatusCreated, cr)
}

// ── Pending Requests ─────────────────────────────────────────────────

// GetPendingRequests returns chill requests received by the user.
// GET /chill/requests
func (h *ChillHandler) GetPendingRequests(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	type requestWithProfile struct {
		models.ChillRequest
		FromUser models.UserProfile `json:"fromUser"`
	}

	var requests []models.ChillRequest
	h.DB.Select(&requests, "SELECT * FROM chill_requests WHERE to_user_id = ? AND status = 'pending' ORDER BY created_at DESC", userID)
	if requests == nil {
		requests = []models.ChillRequest{}
	}

	var result []requestWithProfile
	for _, req := range requests {
		var profile models.UserProfile
		h.DB.Get(&profile, "SELECT id, display_name, bio, profile_photo_url, interests, current_lat, current_lng, current_city, age, is_discoverable FROM users WHERE id = ?", req.FromUserID)
		result = append(result, requestWithProfile{ChillRequest: req, FromUser: profile})
	}

	if result == nil {
		result = []requestWithProfile{}
	}
	writeJSON(w, http.StatusOK, result)
}

// RespondToRequest accepts or declines a chill request.
// PATCH /chill/requests/{id}
func (h *ChillHandler) RespondToRequest(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	requestID := chi.URLParam(r, "id")

	var body struct {
		Accept bool `json:"accept"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	var req models.ChillRequest
	err := h.DB.Get(&req, "SELECT * FROM chill_requests WHERE id = ? AND to_user_id = ? AND status = 'pending'", requestID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Request not found")
		return
	}

	if body.Accept {
		// MATCH!
		h.createMatch(w, req.FromUserID, userID, requestID)
	} else {
		h.DB.Exec("UPDATE chill_requests SET status = 'declined' WHERE id = ?", requestID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "declined"})
	}
}

// ── Matches ──────────────────────────────────────────────────────────

// GetMatches returns all chill matches for the user.
// GET /chill/matches
func (h *ChillHandler) GetMatches(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var matches []models.ChillMatch
	h.DB.Select(&matches, `
		SELECT * FROM chill_matches
		WHERE (user1_id = ? OR user2_id = ?) AND status != 'cancelled'
		ORDER BY created_at DESC`, userID, userID)

	var result []models.ChillMatchWithProfile
	for _, m := range matches {
		otherID := m.User1ID
		if otherID == userID {
			otherID = m.User2ID
		}
		var profile models.UserProfile
		h.DB.Get(&profile, "SELECT id, display_name, bio, profile_photo_url, interests, current_lat, current_lng, current_city, age, is_discoverable FROM users WHERE id = ?", otherID)
		result = append(result, models.ChillMatchWithProfile{ChillMatch: m, OtherUser: profile})
	}

	if result == nil {
		result = []models.ChillMatchWithProfile{}
	}
	writeJSON(w, http.StatusOK, result)
}

// ScheduleMeet sets the date for a chill hangout.
// PATCH /chill/matches/{id}/schedule
func (h *ChillHandler) ScheduleMeet(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	matchID := chi.URLParam(r, "id")

	var match models.ChillMatch
	err := h.DB.Get(&match, "SELECT * FROM chill_matches WHERE id = ? AND (user1_id = ? OR user2_id = ?)", matchID, userID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Match not found")
		return
	}

	var req scheduleMeetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.MeetDate != nil {
		meetTime, err := time.Parse(time.RFC3339, *req.MeetDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid date format. Use ISO 8601.")
			return
		}
		h.DB.Exec("UPDATE chill_matches SET meet_date = ?, status = 'scheduled' WHERE id = ?", meetTime, matchID)
		match.MeetDate = &meetTime
		match.Status = "scheduled"
	}

	writeJSON(w, http.StatusOK, match)
}

// ── Internal: Create Match + Calculate Midpoint ──────────────────────

func (h *ChillHandler) createMatch(w http.ResponseWriter, user1ID, user2ID, requestID string) {
	// Update request status
	h.DB.Exec("UPDATE chill_requests SET status = 'accepted' WHERE id = ?", requestID)
	// Also accept reverse if exists
	h.DB.Exec("UPDATE chill_requests SET status = 'accepted' WHERE from_user_id = ? AND to_user_id = ?", user1ID, user2ID)

	// Get both users' locations
	var user1, user2 models.UserProfile
	h.DB.Get(&user1, "SELECT id, display_name, bio, profile_photo_url, interests, current_lat, current_lng, current_city, age, is_discoverable FROM users WHERE id = ?", user1ID)
	h.DB.Get(&user2, "SELECT id, display_name, bio, profile_photo_url, interests, current_lat, current_lng, current_city, age, is_discoverable FROM users WHERE id = ?", user2ID)

	// Calculate midpoint
	var meetLat, meetLng *float64
	var meetName *string
	if user1.CurrentLat != nil && user1.CurrentLng != nil && user2.CurrentLat != nil && user2.CurrentLng != nil {
		midLat, midLng := midpoint(*user1.CurrentLat, *user1.CurrentLng, *user2.CurrentLat, *user2.CurrentLng)
		meetLat = &midLat
		meetLng = &midLng

		// Generate a friendly meeting location name
		name := "Halfway Point"
		if user1.CurrentCity != nil && user2.CurrentCity != nil {
			if *user1.CurrentCity == *user2.CurrentCity {
				name = "Meet in " + *user1.CurrentCity
			} else {
				name = "Between " + *user1.CurrentCity + " & " + *user2.CurrentCity
			}
		}
		meetName = &name

		// Find nearest experience to the midpoint
		var nearestExp struct {
			ID   string  `db:"id"`
			Dist float64 `db:"dist"`
		}
		err := h.DB.Get(&nearestExp, `
			SELECT id,
				((latitude - ?) * (latitude - ?) + (longitude - ?) * (longitude - ?)) as dist
			FROM experiences
			WHERE latitude IS NOT NULL AND longitude IS NOT NULL AND is_active = 1
			ORDER BY dist ASC LIMIT 1`, midLat, midLat, midLng, midLng)
		if err == nil {
			// Suggest this experience as the chill spot
			_ = nearestExp.ID
		}
	}

	match := models.ChillMatch{
		ID:               uuid.New().String(),
		User1ID:          user1ID,
		User2ID:          user2ID,
		Status:           "matched",
		MeetLat:          meetLat,
		MeetLng:          meetLng,
		MeetLocationName: meetName,
		CreatedAt:        time.Now(),
	}

	h.DB.Exec(`INSERT INTO chill_matches
		(id, user1_id, user2_id, status, meet_lat, meet_lng, meet_location_name, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		match.ID, match.User1ID, match.User2ID, match.Status,
		match.MeetLat, match.MeetLng, match.MeetLocationName, match.CreatedAt)

	// Return match with other user's profile
	otherUser := user2
	result := models.ChillMatchWithProfile{ChillMatch: match, OtherUser: otherUser}
	writeJSON(w, http.StatusCreated, result)
}

// ── Travel Plans ─────────────────────────────────────────────────────

type travelPlanRequest struct {
	Destination    string   `json:"destination"`
	DestinationLat *float64 `json:"destinationLat"`
	DestinationLng *float64 `json:"destinationLng"`
	Country        *string  `json:"country"`
	ArriveDate     string   `json:"arriveDate"`
	DepartDate     string   `json:"departDate"`
	Notes          *string  `json:"notes"`
}

// CreateTravelPlan adds a trip the user is planning.
// POST /chill/travel-plans
func (h *ChillHandler) CreateTravelPlan(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req travelPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Destination == "" || req.ArriveDate == "" || req.DepartDate == "" {
		writeError(w, http.StatusBadRequest, "destination, arriveDate, and departDate are required")
		return
	}

	plan := models.TravelPlan{
		ID: uuid.New().String(), UserID: userID, Destination: req.Destination,
		DestinationLat: req.DestinationLat, DestinationLng: req.DestinationLng,
		Country: req.Country, ArriveDate: req.ArriveDate, DepartDate: req.DepartDate,
		Notes: req.Notes, IsActive: true, CreatedAt: time.Now(),
	}
	_, err := h.DB.Exec(`INSERT INTO travel_plans (id,user_id,destination,destination_lat,destination_lng,country,arrive_date,depart_date,notes,is_active,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		plan.ID, plan.UserID, plan.Destination, plan.DestinationLat, plan.DestinationLng,
		plan.Country, plan.ArriveDate, plan.DepartDate, plan.Notes, plan.IsActive, plan.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create travel plan")
		return
	}
	writeJSON(w, http.StatusCreated, plan)
}

// GetMyTravelPlans returns the user's travel plans.
// GET /chill/travel-plans
func (h *ChillHandler) GetMyTravelPlans(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var plans []models.TravelPlan
	h.DB.Select(&plans, "SELECT * FROM travel_plans WHERE user_id = ? AND is_active = 1 ORDER BY arrive_date ASC", userID)
	if plans == nil { plans = []models.TravelPlan{} }
	writeJSON(w, http.StatusOK, plans)
}

// DeleteTravelPlan removes a travel plan.
// DELETE /chill/travel-plans/{id}
func (h *ChillHandler) DeleteTravelPlan(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	planID := chi.URLParam(r, "id")
	result, _ := h.DB.Exec("DELETE FROM travel_plans WHERE id = ? AND user_id = ?", planID, userID)
	rows, _ := result.RowsAffected()
	if rows == 0 { writeError(w, http.StatusNotFound, "Travel plan not found"); return }
	writeJSON(w, http.StatusOK, map[string]string{"deleted": planID})
}

// DiscoverByTravel finds users with overlapping travel dates at the same destination.
// GET /chill/discover/nearby?destination=X&arrive=YYYY-MM-DD&depart=YYYY-MM-DD
func (h *ChillHandler) DiscoverByTravel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	dest := r.URL.Query().Get("destination")
	arrive := r.URL.Query().Get("arrive")
	depart := r.URL.Query().Get("depart")

	if dest == "" || arrive == "" || depart == "" {
		writeError(w, http.StatusBadRequest, "destination, arrive, and depart query params required")
		return
	}

	// Find overlapping travel plans: their arrive <= my depart AND their depart >= my arrive
	var plans []models.TravelPlan
	h.DB.Select(&plans, `
		SELECT tp.* FROM travel_plans tp
		WHERE tp.user_id != ?
		AND tp.is_active = 1
		AND (tp.destination LIKE ? OR tp.country LIKE ?)
		AND tp.arrive_date <= ?
		AND tp.depart_date >= ?
		AND tp.user_id NOT IN (SELECT user1_id FROM chill_matches WHERE user2_id = ?)
		AND tp.user_id NOT IN (SELECT user2_id FROM chill_matches WHERE user1_id = ?)
		ORDER BY tp.arrive_date ASC`,
		userID, "%"+dest+"%", "%"+dest+"%", depart, arrive, userID, userID)

	var result []models.TravelMatchProfile
	for _, plan := range plans {
		var profile models.UserProfile
		err := h.DB.Get(&profile, "SELECT id, display_name, bio, profile_photo_url, interests, current_lat, current_lng, current_city, age, is_discoverable FROM users WHERE id = ?", plan.UserID)
		if err == nil {
			result = append(result, models.TravelMatchProfile{UserProfile: profile, TravelPlan: plan})
		}
	}
	if result == nil { result = []models.TravelMatchProfile{} }
	writeJSON(w, http.StatusOK, result)
}

// midpoint calculates the geographic midpoint between two coordinates.
func midpoint(lat1, lng1, lat2, lng2 float64) (float64, float64) {
	lat1R := lat1 * math.Pi / 180
	lat2R := lat2 * math.Pi / 180
	lng1R := lng1 * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	bx := math.Cos(lat2R) * math.Cos(dLng)
	by := math.Cos(lat2R) * math.Sin(dLng)

	midLat := math.Atan2(
		math.Sin(lat1R)+math.Sin(lat2R),
		math.Sqrt((math.Cos(lat1R)+bx)*(math.Cos(lat1R)+bx)+by*by),
	) * 180 / math.Pi

	midLng := (lng1R + math.Atan2(by, math.Cos(lat1R)+bx)) * 180 / math.Pi

	return midLat, midLng
}
