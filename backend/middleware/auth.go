package middleware

import (
	"context"
	"net/http"
	"strings"

	"backend/config"
	"backend/db"
	"backend/db/sqlc"
	"backend/security"
	"backend/state"
)

type contextKey string

// UserContextKey is the key used to store the *sqlc.User object in the request context.
const UserContextKey = contextKey("current_user")

// AuthMiddleware validates the JWT token from Cookie or Header, decodes the token, and loads
// user information from PostgreSQL into the Request Context.
func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""

			// 1. Check token from "access_token" Cookie (highest priority for Web Frontend)
			if cookie, err := r.Cookie("access_token"); err == nil {
				tokenString = cookie.Value
			}

			// 2. If Cookie is empty, check "Authorization: Bearer <token>" header (for API Clients/Postman)
			if tokenString == "" {
				authHeader := r.Header.Get("Authorization")
				if authHeader != "" {
					tokenString = authHeader
				}
			}

			if tokenString != "" {
				if strings.HasPrefix(tokenString, "Bearer ") {
					tokenString = strings.TrimPrefix(tokenString, "Bearer ")
				}

				// Validate JWT token against the secret key
				claims, err := security.ValidateAccessToken(tokenString, cfg.SecretKey)
				if err == nil && claims.Username != "" {
					user, ok := lookupUser(r, claims.Username)
					if ok {
						ctx := context.WithValue(r.Context(), UserContextKey, &user)
						r = r.WithContext(ctx)

						// Update online status to Redis (TTL 60s), do not block the request.
						//
						// Previously this used r.Context(), so it was both on the request's
						// critical path and could be cancelled mid-write if the client
						// disconnected — a secondary indicator that did not deserve either.
						go state.TouchUserOnline(context.WithoutCancel(r.Context()), user.ID)
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// lookupUser fetches the user record, preferring a short-lived cache before querying PostgreSQL.
func lookupUser(r *http.Request, username string) (sqlc.User, bool) {
	if cached, ok := globalUserCache.get(username); ok {
		return cached, true
	}

	user, err := db.Queries.GetUserByUsername(r.Context(), username)
	if err != nil {
		return sqlc.User{}, false
	}
	globalUserCache.put(username, user)
	return user, true
}

// GetCurrentUser retrieves the *sqlc.User object stored in the Request Context by AuthMiddleware.
func GetCurrentUser(r *http.Request) (*sqlc.User, bool) {
	user, ok := r.Context().Value(UserContextKey).(*sqlc.User)
	return user, ok
}

// RequireAuth middleware requires the user to be properly authenticated before proceeding.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := GetCurrentUser(r)
		if !ok {
			http.Error(w, `{"detail":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireActiveUser middleware requires the user to be authenticated AND the account to have
// been approved by an Admin (IsApproved = true).
func RequireActiveUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetCurrentUser(r)
		if !ok {
			http.Error(w, `{"detail":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}
		if !user.IsApproved {
			http.Error(w, `{"detail":"Inactive or unapproved user"}`, http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin middleware requires the user to have administrator privileges (Role = "admin").
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetCurrentUser(r)
		if !ok {
			http.Error(w, `{"detail":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}
		if user.Role != "admin" {
			http.Error(w, `{"detail":"Not enough permissions"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
