package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type facebookLoginRequest struct {
	AccessToken string `json:"accessToken"`
}

type facebookUser struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Picture struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	} `json:"picture"`
}

// FacebookLogin handles POST /auth/facebook
// Validates Facebook access token, finds or creates user, returns JWT.
func (h *AuthHandler) FacebookLogin(w http.ResponseWriter, r *http.Request) {
	var req facebookLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.AccessToken == "" {
		writeError(w, http.StatusBadRequest, "accessToken is required")
		return
	}

	// Validate token with Facebook Graph API
	fbURL := fmt.Sprintf("https://graph.facebook.com/me?fields=id,name,email,picture.type(large)&access_token=%s", req.AccessToken)
	resp, err := http.Get(fbURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to contact Facebook")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		writeError(w, http.StatusUnauthorized, "Invalid Facebook token")
		return
	}

	var fbUser facebookUser
	if err := json.NewDecoder(resp.Body).Decode(&fbUser); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to parse Facebook response")
		return
	}

	if fbUser.ID == "" {
		writeError(w, http.StatusUnauthorized, "Invalid Facebook user")
		return
	}

	// Find existing user by facebook_id
	var userID, walletAddr string
	var isAdmin bool
	err = h.DB.QueryRow("SELECT id, wallet_address, is_admin FROM users WHERE facebook_id = ?", fbUser.ID).Scan(&userID, &walletAddr, &isAdmin)

	if err != nil {
		// Try by email
		if fbUser.Email != "" {
			err = h.DB.QueryRow("SELECT id, wallet_address, is_admin FROM users WHERE email = ?", fbUser.Email).Scan(&userID, &walletAddr, &isAdmin)
		}
	}

	if err != nil {
		// Create new user
		userID = uuid.New().String()
		walletAddr = "0x" + uuid.New().String()[:40]
		now := time.Now()

		photoURL := fbUser.Picture.Data.URL

		_, err = h.DB.Exec(`INSERT INTO users
			(id, wallet_address, display_name, email, profile_photo_url, facebook_id, auth_method, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 'facebook', ?, ?)`,
			userID, walletAddr, fbUser.Name, fbUser.Email, photoURL, fbUser.ID, now, now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to create account")
			return
		}
	} else {
		// Update existing user with Facebook info
		h.DB.Exec("UPDATE users SET facebook_id = ?, display_name = COALESCE(display_name, ?), profile_photo_url = COALESCE(profile_photo_url, ?) WHERE id = ?",
			fbUser.ID, fbUser.Name, fbUser.Picture.Data.URL, userID)
	}

	token, err := h.generateToken(userID, walletAddr, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":  token,
		"userId": userID,
		"email":  fbUser.Email,
		"name":   fbUser.Name,
	})
}
