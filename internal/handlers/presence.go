package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/jmoiron/sqlx"
)

type PresenceHandler struct {
	DB *sqlx.DB
}

type updatePresenceRequest struct {
	Latitude          *float64 `json:"latitude"`
	Longitude         *float64 `json:"longitude"`
	City              *string  `json:"city"`
	Spot              *string  `json:"spot"`
	PreciseLocationOk *bool    `json:"preciseLocationOk"`
	Discoverable      *bool    `json:"isDiscoverable"`
}

type presenceProfile struct {
	ID                string     `json:"id" db:"id"`
	DisplayName       *string    `json:"displayName" db:"display_name"`
	CurrentCity       *string    `json:"currentCity" db:"current_city"`
	CurrentSpot       *string    `json:"currentSpot" db:"current_spot"`
	CurrentLat        *float64   `json:"currentLat" db:"current_lat"`
	CurrentLng        *float64   `json:"currentLng" db:"current_lng"`
	PresenceUpdatedAt *time.Time `json:"presenceUpdatedAt" db:"presence_updated_at"`
}

type nearbyPresenceResponse struct {
	Count    int               `json:"count"`
	RadiusKm float64           `json:"radiusKm"`
	People   []presenceProfile `json:"people"`
}

type presenceHub struct {
	Key           string   `json:"key"`
	Label         string   `json:"label"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
	People        int      `json:"people"`
	Opportunities int      `json:"opportunities"`
}

// UpdatePresence records lightweight, expiring presence for nearby counts.
// PUT /presence
func (h *PresenceHandler) UpdatePresence(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req updatePresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if (req.Latitude == nil) != (req.Longitude == nil) {
		writeError(w, http.StatusBadRequest, "latitude and longitude must be sent together")
		return
	}
	if req.Latitude != nil && (*req.Latitude < -90 || *req.Latitude > 90) {
		writeError(w, http.StatusBadRequest, "latitude is out of range")
		return
	}
	if req.Longitude != nil && (*req.Longitude < -180 || *req.Longitude > 180) {
		writeError(w, http.StatusBadRequest, "longitude is out of range")
		return
	}

	now := time.Now().UTC()
	expires := now.Add(45 * time.Minute)
	var err error
	if req.Latitude != nil && req.Longitude != nil && req.City == nil && req.Spot == nil {
		_, err = h.DB.Exec(`UPDATE users SET
			current_lat = ?,
			current_lng = ?,
			current_city = NULL,
			current_spot = NULL,
			current_spot_lat = NULL,
			current_spot_lng = NULL,
			precise_location_ok = COALESCE(?, precise_location_ok),
			is_discoverable = COALESCE(?, is_discoverable),
			presence_updated_at = ?,
			presence_expires_at = ?,
			updated_at = ?
			WHERE id = ?`,
			req.Latitude, req.Longitude, req.PreciseLocationOk, req.Discoverable,
			now, expires, now, userID)
	} else {
		_, err = h.DB.Exec(`UPDATE users SET
			current_lat = COALESCE(?, current_lat),
			current_lng = COALESCE(?, current_lng),
			current_city = COALESCE(?, current_city),
			current_spot = COALESCE(?, current_spot),
			current_spot_lat = COALESCE(?, current_spot_lat),
			current_spot_lng = COALESCE(?, current_spot_lng),
			precise_location_ok = COALESCE(?, precise_location_ok),
			is_discoverable = COALESCE(?, is_discoverable),
			presence_updated_at = ?,
			presence_expires_at = ?,
			updated_at = ?
			WHERE id = ?`,
			req.Latitude, req.Longitude, req.City, req.Spot,
			req.Latitude, req.Longitude, req.PreciseLocationOk, req.Discoverable,
			now, expires, now, userID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update presence")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"presenceUpdatedAt": now,
		"presenceExpiresAt": expires,
	})
}

// Nearby returns discoverable users within a radius from lat/lng.
// GET /presence/nearby?lat=19.43&lng=-99.13&radiusKm=75
func (h *PresenceHandler) Nearby(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "lat is required")
		return
	}
	lng, err := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "lng is required")
		return
	}
	radiusKm := 75.0
	if raw := r.URL.Query().Get("radiusKm"); raw != "" {
		if parsed, parseErr := strconv.ParseFloat(raw, 64); parseErr == nil && parsed > 0 && parsed <= 500 {
			radiusKm = parsed
		}
	}

	var profiles []presenceProfile
	err = h.DB.Select(&profiles, `SELECT id, display_name, current_city, current_spot, current_lat, current_lng, presence_updated_at
		FROM users
		WHERE id != ?
		AND is_discoverable = 1
		AND current_lat IS NOT NULL
		AND current_lng IS NOT NULL
		AND (presence_expires_at IS NULL OR presence_expires_at > ?)`,
		userID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load nearby people")
		return
	}

	center := geoPoint{Lat: lat, Lng: lng}
	nearby := make([]presenceProfile, 0)
	for _, profile := range profiles {
		if profile.CurrentLat == nil || profile.CurrentLng == nil {
			continue
		}
		if haversineKm(center, geoPoint{Lat: *profile.CurrentLat, Lng: *profile.CurrentLng}) <= radiusKm {
			nearby = append(nearby, profile)
		}
	}

	writeJSON(w, http.StatusOK, nearbyPresenceResponse{
		Count:    len(nearby),
		RadiusKm: radiusKm,
		People:   nearby,
	})
}

// Hubs returns city/location-level activity for the world map.
// GET /presence/hubs
func (h *PresenceHandler) Hubs(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Label     string   `db:"label"`
		Latitude  *float64 `db:"latitude"`
		Longitude *float64 `db:"longitude"`
		Count     int      `db:"count"`
	}

	var peopleRows []row
		err := h.DB.Select(&peopleRows, `SELECT
			COALESCE(NULLIF(current_spot, ''), 'Live location') AS label,
			AVG(current_lat) AS latitude,
			AVG(current_lng) AS longitude,
			COUNT(*) AS count
		FROM users
		WHERE is_discoverable = 1
		AND (presence_expires_at IS NULL OR presence_expires_at > ?)
		AND (current_spot IS NOT NULL OR current_lat IS NOT NULL)
		GROUP BY label`,
		time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load people hubs")
		return
	}

	var opportunityRows []row
	err = h.DB.Select(&opportunityRows, `SELECT
			COALESCE(NULLIF(location_name, ''), 'Hosted locally') AS label,
			AVG(latitude) AS latitude,
			AVG(longitude) AS longitude,
			COUNT(*) AS count
		FROM experiences
		WHERE is_active = 1 AND status = 'active'
		GROUP BY label`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load opportunity hubs")
		return
	}

	hubs := map[string]*presenceHub{}
	for _, item := range peopleRows {
		key := item.Label
		hubs[key] = &presenceHub{Key: key, Label: item.Label, Latitude: item.Latitude, Longitude: item.Longitude, People: item.Count}
	}
	for _, item := range opportunityRows {
		key := item.Label
		hub := hubs[key]
		if hub == nil {
			hub = &presenceHub{Key: key, Label: item.Label, Latitude: item.Latitude, Longitude: item.Longitude}
			hubs[key] = hub
		}
		hub.Opportunities += item.Count
		if hub.Latitude == nil {
			hub.Latitude = item.Latitude
			hub.Longitude = item.Longitude
		}
	}

	result := make([]presenceHub, 0, len(hubs))
	for _, hub := range hubs {
		result = append(result, *hub)
	}
	writeJSON(w, http.StatusOK, result)
}

type geoPoint struct {
	Lat float64
	Lng float64
}

func haversineKm(a, b geoPoint) float64 {
	const earthKm = 6371
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	x := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Sin(dLng/2)*math.Sin(dLng/2)*math.Cos(lat1)*math.Cos(lat2)
	return earthKm * 2 * math.Atan2(math.Sqrt(x), math.Sqrt(1-x))
}
