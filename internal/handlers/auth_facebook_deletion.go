package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// FacebookDataDeletionRequest is the JSON payload of a Facebook signed_request
// after it has been verified and decoded.
type facebookSignedPayload struct {
	UserID    string `json:"user_id"`
	Algorithm string `json:"algorithm"`
	IssuedAt  int64  `json:"issued_at"`
}

// FacebookDataDeletion handles POST /auth/facebook/data-deletion
//
// Facebook calls this endpoint when a user removes the EmpowerTours app from
// their Facebook account. The request body contains a single form field
// `signed_request` of the form `<sig>.<base64url payload>`. We verify the
// HMAC-SHA256 signature using the Facebook app secret, then delete the user
// row keyed by facebook_id and respond with the required JSON envelope.
//
// Docs: https://developers.facebook.com/docs/development/create-an-app/app-dashboard/data-deletion-callback/
func (h *AuthHandler) FacebookDataDeletion(w http.ResponseWriter, r *http.Request) {
	if h.FacebookAppSecret == "" {
		writeError(w, http.StatusInternalServerError, "Facebook app secret not configured")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid form body")
		return
	}

	signed := r.FormValue("signed_request")
	if signed == "" {
		writeError(w, http.StatusBadRequest, "signed_request is required")
		return
	}

	payload, ok := parseFacebookSignedRequest(signed, h.FacebookAppSecret)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Invalid signed_request")
		return
	}

	if payload.UserID == "" {
		writeError(w, http.StatusBadRequest, "Missing user_id in signed_request")
		return
	}

	// Delete the user record (and any rows that cascade from it).
	// We do not delete payments — those are governed by the payment processor.
	if _, err := h.DB.Exec("DELETE FROM users WHERE facebook_id = ?", payload.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	// Confirmation code Facebook will display to the user. Use the FB user ID
	// suffix so the user can quote it back to support.
	code := "del_" + payload.UserID

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":               "https://api.empowertours.xyz/data-deletion?code=" + code,
		"confirmation_code": code,
	})
}

// parseFacebookSignedRequest verifies the HMAC-SHA256 signature on a
// Facebook signed_request string and returns the decoded payload. Returns
// ok=false if the signature is invalid or the payload cannot be decoded.
func parseFacebookSignedRequest(signed, appSecret string) (facebookSignedPayload, bool) {
	var out facebookSignedPayload

	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return out, false
	}
	encodedSig, encodedPayload := parts[0], parts[1]

	sig, err := base64URLDecode(encodedSig)
	if err != nil {
		return out, false
	}

	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte(encodedPayload))
	expected := mac.Sum(nil)

	if !hmac.Equal(sig, expected) {
		return out, false
	}

	payloadBytes, err := base64URLDecode(encodedPayload)
	if err != nil {
		return out, false
	}

	if err := json.Unmarshal(payloadBytes, &out); err != nil {
		return out, false
	}
	if !strings.EqualFold(out.Algorithm, "HMAC-SHA256") {
		return out, false
	}
	return out, true
}

// base64URLDecode handles Facebook's URL-safe base64 (no padding).
func base64URLDecode(s string) ([]byte, error) {
	// Try padded URL encoding first.
	if b, err := base64.URLEncoding.DecodeString(padBase64(s)); err == nil {
		return b, nil
	}
	// Fall back to raw URL encoding.
	return base64.RawURLEncoding.DecodeString(s)
}

func padBase64(s string) string {
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return s
}

