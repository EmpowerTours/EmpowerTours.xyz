package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	mw "github.com/empowertours/empowertours-app/internal/middleware"
)

// AuthHandler holds dependencies for authentication endpoints.
type AuthHandler struct {
	DB                *sqlx.DB
	JWTSecret         string
	FacebookAppSecret string
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db *sqlx.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{DB: db, JWTSecret: jwtSecret}
}

type nonceRequest struct {
	WalletAddress string `json:"walletAddress"`
}

type nonceResponse struct {
	Nonce string `json:"nonce"`
}

type loginRequest struct {
	WalletAddress string `json:"walletAddress"`
	Signature     string `json:"signature"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// Nonce handles POST /auth/nonce — generates a random nonce for wallet signing.
func (h *AuthHandler) Nonce(w http.ResponseWriter, r *http.Request) {
	var req nonceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	addr := strings.TrimSpace(req.WalletAddress)
	if addr == "" || !common.IsHexAddress(addr) {
		writeError(w, http.StatusBadRequest, "Invalid wallet address")
		return
	}

	// Normalize to checksummed address
	addr = common.HexToAddress(addr).Hex()

	// Generate random nonce
	nonce, err := generateNonce()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate nonce")
		return
	}

	// Upsert user: create if not exists, always update nonce
	var existingID string
	err = h.DB.GetContext(r.Context(), &existingID,
		"SELECT id FROM users WHERE wallet_address = ?", addr)

	if err == sql.ErrNoRows {
		// Create new user
		id := uuid.New().String()
		_, err = h.DB.ExecContext(r.Context(),
			`INSERT INTO users (id, wallet_address, nonce, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)`,
			id, addr, nonce, time.Now().UTC(), time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to create user")
			return
		}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	} else {
		// Update nonce for existing user
		_, err = h.DB.ExecContext(r.Context(),
			"UPDATE users SET nonce = ?, updated_at = ? WHERE wallet_address = ?",
			nonce, time.Now().UTC(), addr)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to update nonce")
			return
		}
	}

	writeJSON(w, http.StatusOK, nonceResponse{Nonce: nonce})
}

// Login handles POST /auth/login — verifies EIP-191 signature and returns a JWT.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	addr := strings.TrimSpace(req.WalletAddress)
	if addr == "" || !common.IsHexAddress(addr) {
		writeError(w, http.StatusBadRequest, "Invalid wallet address")
		return
	}

	addr = common.HexToAddress(addr).Hex()

	// Look up user and nonce
	var user struct {
		ID      string  `db:"id"`
		Nonce   *string `db:"nonce"`
		IsAdmin bool    `db:"is_admin"`
	}
	err := h.DB.GetContext(r.Context(), &user,
		"SELECT id, nonce, is_admin FROM users WHERE wallet_address = ?", addr)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "Request a nonce first")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if user.Nonce == nil || *user.Nonce == "" {
		writeError(w, http.StatusUnauthorized, "Request a nonce first")
		return
	}

	// Verify EIP-191 personal_sign signature
	if !verifyPersonalSign(addr, *user.Nonce, req.Signature) {
		writeError(w, http.StatusUnauthorized, "Invalid signature")
		return
	}

	// Regenerate nonce to prevent replay attacks
	newNonce, _ := generateNonce()
	_, _ = h.DB.ExecContext(r.Context(),
		"UPDATE users SET nonce = ?, updated_at = ? WHERE id = ?",
		newNonce, time.Now().UTC(), user.ID)

	// Generate JWT
	claims := mw.Claims{
		UserID:        user.ID,
		WalletAddress: addr,
		IsAdmin:       user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "empowertours",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(h.JWTSecret))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{Token: tokenStr})
}

// verifyPersonalSign verifies an EIP-191 personal_sign signature.
// The message is hashed as: keccak256("\x19Ethereum Signed Message:\n" + len(message) + message)
func verifyPersonalSign(address, message, sigHex string) bool {
	sig, err := hex.DecodeString(strings.TrimPrefix(sigHex, "0x"))
	if err != nil || len(sig) != 65 {
		return false
	}

	// Adjust recovery ID: MetaMask uses 27/28, go-ethereum expects 0/1
	if sig[64] >= 27 {
		sig[64] -= 27
	}

	// EIP-191 prefix
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	hash := crypto.Keccak256Hash([]byte(prefixed))

	pubKey, err := crypto.SigToPub(hash.Bytes(), sig)
	if err != nil {
		return false
	}

	recoveredAddr := crypto.PubkeyToAddress(*pubKey)
	return strings.EqualFold(recoveredAddr.Hex(), address)
}

// generateNonce creates a random 32-byte hex nonce.
func generateNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

