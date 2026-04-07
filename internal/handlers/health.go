package handlers

import "net/http"

// HealthCheck returns a simple health status.
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"data":{"status":"ok","version":"0.1.0"}}`))
}
