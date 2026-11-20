package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeliverRelaySignsPayload(t *testing.T) {
	secret := strings.Repeat("k", 40)
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(r.Header.Get("X-Relay-Ts") + "\n"))
		mac.Write(body)
		if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(r.Header.Get("X-Relay-Sig"))) {
			http.Error(w, `{"error":"signature"}`, http.StatusUnauthorized)
			return
		}
		json.Unmarshal(body, &got)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := SiteSMTP{To: "owner@example.com", RelayURL: srv.URL, RelaySecret: secret}
	if !s.ready() {
		t.Fatal("relay config should be ready without SMTP fields")
	}
	if err := s.deliver("Site", "client@example.com", "owner@example.com", "Hola ñ", "line1\nline2"); err != nil {
		t.Fatal(err)
	}
	if got["to"] != "client@example.com" || got["subject"] != "Hola ñ" || got["body"] != "line1\nline2" {
		t.Fatalf("unexpected payload %v", got)
	}

	bad := SiteSMTP{To: "owner@example.com", RelayURL: srv.URL, RelaySecret: strings.Repeat("x", 40)}
	if err := bad.deliver("Site", "client@example.com", "", "s", "b"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("wrong secret should fail with 401, got %v", err)
	}
	if err := s.deliver("Site", "a@example.com\r\nBcc: x@example.com", "", "s", "b"); err == nil {
		t.Fatal("header injection must be rejected before the relay")
	}
	if (SiteSMTP{To: "o@example.com", RelayURL: srv.URL, RelaySecret: "short"}).ready() {
		t.Fatal("short secret must not enable the relay")
	}
}
