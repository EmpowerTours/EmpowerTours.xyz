package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/empowertours/empowertours-app/internal/config"
)

// GoogleReceiptValidator validates Google Play purchase tokens.
type GoogleReceiptValidator struct {
	cfg     *config.Config
	enabled bool
}

// GooglePurchaseInfo holds verified purchase data from Google Play.
type GooglePurchaseInfo struct {
	OrderID          string `json:"orderId"`
	PurchaseState    int    `json:"purchaseState"`
	ExpiryTimeMillis int64  `json:"expiryTimeMillis"`
	AutoRenewing     bool   `json:"autoRenewing"`
}

func NewGoogleReceiptValidator(cfg *config.Config) *GoogleReceiptValidator {
	enabled := cfg.GooglePlayCredentialsJSON != ""
	if !enabled {
		log.Println("Warning: Google Play receipt validation disabled (missing GOOGLE_PLAY_CREDENTIALS_JSON)")
	}
	return &GoogleReceiptValidator{cfg: cfg, enabled: enabled}
}

// Validate verifies a Google Play subscription purchase token.
// If validation is disabled (dev mode), returns nil info and nil error.
func (v *GoogleReceiptValidator) Validate(packageName, subscriptionID, purchaseToken string) (*GooglePurchaseInfo, error) {
	if !v.enabled {
		return nil, nil
	}

	// Use REST API directly to avoid heavy google API client dependency
	url := fmt.Sprintf(
		"https://androidpublisher.googleapis.com/androidpublisher/v3/applications/%s/purchases/subscriptions/%s/tokens/%s",
		packageName, subscriptionID, purchaseToken,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// In production, use OAuth2 service account token
	// For now, log that we'd need proper auth
	log.Printf("Google Play verification for package=%s sub=%s", packageName, subscriptionID)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Play API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Google Play API returned %d: %s", resp.StatusCode, string(body))
	}

	var info GooglePurchaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode Google Play response: %w", err)
	}

	// PurchaseState: 0=purchased, 1=canceled, 2=pending
	if info.PurchaseState != 0 {
		return nil, fmt.Errorf("purchase not in valid state: %d", info.PurchaseState)
	}

	return &info, nil
}
