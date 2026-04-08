package services

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/empowertours/empowertours-app/internal/config"
)

// AppleReceiptValidator validates App Store Server API v2 signed transactions.
type AppleReceiptValidator struct {
	cfg     *config.Config
	enabled bool
}

// AppleTransactionInfo holds decoded fields from a JWS signed transaction.
type AppleTransactionInfo struct {
	TransactionID         string `json:"transactionId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	PurchaseDate          int64  `json:"purchaseDate"`
	ExpiresDate           int64  `json:"expiresDate"`
	InAppOwnershipType    string `json:"inAppOwnershipType"`
	Environment           string `json:"environment"`
}

func NewAppleReceiptValidator(cfg *config.Config) *AppleReceiptValidator {
	enabled := cfg.AppleIssuerID != "" && cfg.AppleKeyID != "" && cfg.ApplePrivateKey != "" && cfg.AppleBundleID != ""
	if !enabled {
		log.Println("Warning: Apple receipt validation disabled (missing APPLE_* env vars)")
	}
	return &AppleReceiptValidator{cfg: cfg, enabled: enabled}
}

// Validate verifies an Apple signed transaction (JWS).
// If validation is disabled (dev mode), returns nil info and nil error.
func (v *AppleReceiptValidator) Validate(signedTransaction string) (*AppleTransactionInfo, error) {
	if !v.enabled {
		return nil, nil
	}

	info, err := v.parseJWSPayload(signedTransaction)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Apple JWS payload: %w", err)
	}

	if info.BundleID != v.cfg.AppleBundleID {
		return nil, fmt.Errorf("bundle ID mismatch: got %q, expected %q", info.BundleID, v.cfg.AppleBundleID)
	}

	if err := v.verifyWithAPI(info.TransactionID); err != nil {
		return nil, fmt.Errorf("App Store API verification failed: %w", err)
	}

	return info, nil
}

func (v *AppleReceiptValidator) parseJWSPayload(jws string) (*AppleTransactionInfo, error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWS format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWS payload: %w", err)
	}

	var info AppleTransactionInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction info: %w", err)
	}

	if info.TransactionID == "" {
		return nil, fmt.Errorf("transactionId is empty in JWS payload")
	}

	return &info, nil
}

func (v *AppleReceiptValidator) verifyWithAPI(transactionID string) error {
	token, err := v.generateAppStoreJWT()
	if err != nil {
		return fmt.Errorf("failed to generate App Store JWT: %w", err)
	}

	urls := []string{
		fmt.Sprintf("https://api.storekit.itunes.apple.com/inApps/v1/transactions/%s", transactionID),
		fmt.Sprintf("https://api.storekit-sandbox.itunes.apple.com/inApps/v1/transactions/%s", transactionID),
	}

	var lastErr error
	for _, url := range urls {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return nil
		}
		lastErr = fmt.Errorf("App Store API returned status %d for %s", resp.StatusCode, url)
	}

	return lastErr
}

func (v *AppleReceiptValidator) generateAppStoreJWT() (string, error) {
	block, _ := pem.Decode([]byte(v.cfg.ApplePrivateKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode Apple private key PEM")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse Apple private key: %w", err)
	}

	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("Apple private key is not ECDSA")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": v.cfg.AppleIssuerID,
		"iat": now.Unix(),
		"exp": now.Add(20 * time.Minute).Unix(),
		"aud": "appstoreconnect-v1",
		"bid": v.cfg.AppleBundleID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = v.cfg.AppleKeyID

	return token.SignedString(ecKey)
}
