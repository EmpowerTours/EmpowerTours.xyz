package middleware

import (
	"net/http"

	"github.com/jmoiron/sqlx"
)

// RequireMembership returns middleware that checks whether the authenticated
// user has a non-NULL, non-empty membership_tier in the database.
// Must be used after JWTAuth middleware so the user ID is available in context.
func RequireMembership(db *sqlx.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				http.Error(w, `{"error":"Authentication required"}`, http.StatusUnauthorized)
				return
			}

			var tier *string
			err := db.GetContext(r.Context(), &tier,
				"SELECT membership_tier FROM users WHERE id = ?", userID)
			if err != nil {
				http.Error(w, `{"error":"User not found"}`, http.StatusUnauthorized)
				return
			}

			if tier == nil || *tier == "" {
				http.Error(w, `{"error":"Membership required"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
