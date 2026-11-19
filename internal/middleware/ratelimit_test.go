package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterIgnoresClientPort(t *testing.T) {
	rl := NewRateLimiter(2, time.Hour)
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	codes := []int{}
	for i, port := range []string{"1111", "2222", "3333"} {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "203.0.113.7:" + port
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		codes = append(codes, w.Code)
		_ = i
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("third request from the same IP on a new port: got %v, want 429 last", codes)
	}
}
