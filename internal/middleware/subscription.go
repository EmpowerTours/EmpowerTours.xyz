package middleware

import (
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

// RequireSubscription blocks access unless the user has an active subscription or trial.
func RequireSubscription(db *sqlx.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			// Check if admin (admins bypass subscription)
			if IsAdmin(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}

			var subStatus *string
			var trialEnds *time.Time
			var subEnds *time.Time

			err := db.QueryRow(
				"SELECT subscription_status, trial_ends_at, subscription_ends_at FROM users WHERE id = ?",
				userID,
			).Scan(&subStatus, &trialEnds, &subEnds)

			if err != nil {
				http.Error(w, `{"error":"user not found"}`, http.StatusUnauthorized)
				return
			}

			now := time.Now()
			hasAccess := false

			if subStatus != nil {
				switch *subStatus {
				case "trialing":
					hasAccess = trialEnds != nil && trialEnds.After(now)
				case "active":
					hasAccess = subEnds != nil && subEnds.After(now)
				}
			}

			if !hasAccess {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				w.Write([]byte(`{"error":"subscription_required","message":"Please subscribe or start a free trial to access EmpowerTours"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
