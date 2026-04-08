package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type emailLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Register handles POST /auth/register — create account with email+password
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if !emailRegex.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "Invalid email address")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}
	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "Display name is required")
		return
	}

	// Check if email already exists
	var count int
	h.DB.Get(&count, "SELECT COUNT(*) FROM users WHERE email = ?", req.Email)
	if count > 0 {
		writeError(w, http.StatusConflict, "An account with this email already exists")
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to process password")
		return
	}

	// Generate a placeholder wallet address for email users
	walletBytes := make([]byte, 20)
	rand.Read(walletBytes)
	walletAddr := "0x" + hex.EncodeToString(walletBytes)

	now := time.Now()
	userID := uuid.New().String()

	_, err = h.DB.Exec(`INSERT INTO users
		(id, wallet_address, display_name, email, password_hash, auth_method, email_verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'email', 0, ?, ?)`,
		userID, walletAddr, req.DisplayName, req.Email, string(hash), now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create account")
		return
	}

	// Generate JWT
	token, err := h.generateToken(userID, walletAddr, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":  token,
		"userId": userID,
		"email":  req.Email,
	})
}

// EmailLogin handles POST /auth/email-login — login with email+password
func (h *AuthHandler) EmailLogin(w http.ResponseWriter, r *http.Request) {
	var req emailLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Email and password are required")
		return
	}

	// Find user
	var userID, walletAddr, passwordHash string
	var isAdmin bool
	err := h.DB.QueryRow(
		"SELECT id, wallet_address, password_hash, is_admin FROM users WHERE email = ? AND auth_method = 'email'",
		req.Email,
	).Scan(&userID, &walletAddr, &passwordHash, &isAdmin)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	// Generate JWT
	token, err := h.generateToken(userID, walletAddr, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":  token,
		"userId": userID,
		"email":  req.Email,
	})
}

// generateToken creates a JWT for the given user.
// Uses the same claim keys as the wallet-based auth (auth.go) so the
// JWTAuth middleware can parse them consistently.
func (h *AuthHandler) generateToken(userID, walletAddr string, isAdmin bool) (string, error) {
	claims := jwt.MapClaims{
		"userId":        userID,
		"walletAddress": walletAddr,
		"isAdmin":       isAdmin,
		"iat":           time.Now().Unix(),
		"exp":           time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.JWTSecret))
}
