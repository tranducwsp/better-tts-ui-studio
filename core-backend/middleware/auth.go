package middleware

import (
	"context"
	"net/http"
	"strings"

	"core-backend/config"
	"core-backend/db"
	"core-backend/models"
	"core-backend/security"
)

type contextKey string

const UserContextKey = contextKey("current_user")

func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""

			// 1. Try Cookie
			if cookie, err := r.Cookie("access_token"); err == nil {
				tokenString = cookie.Value
			}

			// 2. Try Authorization Header if no cookie
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

				claims, err := security.ValidateToken(tokenString, cfg.SecretKey)
				if err == nil && claims.Username != "" {
					var user models.User
					if err := db.DB.Where("username = ?", claims.Username).First(&user).Error; err == nil {
						ctx := context.WithValue(r.Context(), UserContextKey, &user)
						r = r.WithContext(ctx)
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func GetCurrentUser(r *http.Request) (*models.User, bool) {
	user, ok := r.Context().Value(UserContextKey).(*models.User)
	return user, ok
}

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
