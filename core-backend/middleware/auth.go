package middleware

import (
	"context"
	"net/http"
	"strings"

	"core-backend/config"
	"core-backend/db"
	"core-backend/db/sqlc"
	"core-backend/security"
)

type contextKey string

// UserContextKey là key được dùng để lưu đối tượng *sqlc.User vào trong request context.
const UserContextKey = contextKey("current_user")

// AuthMiddleware kiểm tra token JWT từ Cookie hoặc Header, giải mã token và nạp thông tin người dùng từ PostgreSQL vào Request Context.
func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""

			// 1. Kiểm tra Token từ Cookie "access_token" (ưu tiên tối đa cho Web Frontend)
			if cookie, err := r.Cookie("access_token"); err == nil {
				tokenString = cookie.Value
			}

			// 2. Nếu Cookie trống, kiểm tra Header "Authorization: Bearer <token>" (cho API Clients/Postman)
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

				// Validate JWT token với secret key
				claims, err := security.ValidateToken(tokenString, cfg.SecretKey)
				if err == nil && claims.Username != "" {
					// Query trạng thái tài khoản thời gian thực từ PostgreSQL bằng sqlc
					user, err := db.Queries.GetUserByUsername(r.Context(), claims.Username)
					if err == nil {
						ctx := context.WithValue(r.Context(), UserContextKey, &user)
						r = r.WithContext(ctx)
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetCurrentUser lấy đối tượng *sqlc.User đã được lưu trong Request Context bởi AuthMiddleware.
func GetCurrentUser(r *http.Request) (*sqlc.User, bool) {
	user, ok := r.Context().Value(UserContextKey).(*sqlc.User)
	return user, ok
}

// RequireAuth middleware yêu cầu người dùng phải đăng nhập hợp lệ trước khi tiếp tục.
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

// RequireActiveUser middleware yêu cầu người dùng đã đăng nhập VÀ tài khoản đã được Admin phê duyệt (IsApproved = true).
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

// RequireAdmin middleware yêu cầu người dùng có quyền quản trị viên (Role = "admin").
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
