package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	contextKeyUserID        contextKey = "userID"
	contextKeyWalletAddress contextKey = "walletAddress"
	contextKeyIsAdmin       contextKey = "isAdmin"
)

// Claims represents the JWT claims for EmpowerTours auth tokens.
type Claims struct {
	UserID        string `json:"userId"`
	WalletAddress string `json:"walletAddress"`
	IsAdmin       bool   `json:"isAdmin"`
	jwt.RegisteredClaims
}

// JWTAuth returns middleware that validates a Bearer JWT token from the
// Authorization header and injects user info into the request context.
func JWTAuth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"Authorization header required"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, `{"error":"Invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := parts[1]
			claims := &Claims{}

			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, `{"error":"Invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, contextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, contextKeyWalletAddress, claims.WalletAddress)
			ctx = context.WithValue(ctx, contextKeyIsAdmin, claims.IsAdmin)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin is middleware that checks the IsAdmin context value set by JWTAuth.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			http.Error(w, `{"error":"Admin access required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserID returns the authenticated user's ID from the context.
func GetUserID(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyUserID).(string)
	return v
}

// GetWalletAddress returns the authenticated user's wallet address from the context.
func GetWalletAddress(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyWalletAddress).(string)
	return v
}

// IsAdmin returns whether the authenticated user is an admin.
func IsAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(contextKeyIsAdmin).(bool)
	return v
}
