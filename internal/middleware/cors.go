package middleware

import (
	"net/http"

	"github.com/go-chi/cors"
)

// CORS returns a configured CORS handler for the EmpowerTours API.
func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins: []string{
			"https://empowertours.xyz",
			"https://www.empowertours.xyz",
			"https://api.empowertours.xyz",
			"https://*.up.railway.app",
			"https://*.exp.direct",
			"http://localhost:3000",
			"http://localhost:8081",
			"http://localhost:8082",
			"http://localhost:19006",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:8081",
			"http://127.0.0.1:8082",
			"http://127.0.0.1:19006",
			"http://192.168.*",
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
