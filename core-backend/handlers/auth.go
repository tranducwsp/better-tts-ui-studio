package handlers

import (
	"net/http"
	"strings"

	"core-backend/config"
	"core-backend/db"
	"core-backend/db/sqlc"
	"core-backend/middleware"
	"core-backend/security"
	"core-backend/state"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AuthHandler xử lý các API liên quan đến Xác thực (Authentication) và Quản lý người dùng.
type AuthHandler struct {
	Config *config.Config
}

// NewAuthHandler khởi tạo một AuthHandler mới với cấu hình hệ thống.
func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{Config: cfg}
}

// UserCreateRequest cấu trúc payload yêu cầu khi đăng ký hoặc đăng nhập.
type UserCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UserResponse cấu trúc dữ liệu người dùng trả về cho client (không chứa PasswordHash).
type UserResponse struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	IsApproved bool   `json:"is_approved"`
	IsOnline   bool   `json:"is_online"`
}

// Register xử lý đăng ký tài khoản mới. Tài khoản đăng ký mới sẽ mặc định ở trạng thái IsApproved = false (chờ Admin duyệt).
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req UserCreateRequest
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") || strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		req.Username = r.FormValue("username")
		req.Password = r.FormValue("password")
	} else {
		if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			req.Username = r.FormValue("username")
			req.Password = r.FormValue("password")
			if req.Username == "" && req.Password == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Invalid JSON payload"})
				return
			}
		}
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Username and password are required"})
		return
	}

	// 1. Check if user already exists via sqlc
	_, err := db.Queries.GetUserByUsername(r.Context(), req.Username)
	if err == nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Username already exists"})
		return
	}

	// 2. Hash password with Bcrypt
	hashedPassword, err := security.HashPassword(req.Password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to hash password"})
		return
	}

	// 3. Create User record in PostgreSQL
	userID := uuid.NewString()
	user, err := db.Queries.CreateUser(r.Context(), sqlc.CreateUserParams{
		ID:           userID,
		Username:     req.Username,
		PasswordHash: hashedPassword,
		Role:         "user",
		IsApproved:   false, // Requires admin approval
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to create account"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Registration successful. Please wait for Administrator approval.",
		"user": UserResponse{
			ID:         user.ID,
			Username:   user.Username,
			Role:       user.Role,
			IsApproved: user.IsApproved,
			IsOnline:   false,
		},
	})
}

// Login handles authentication, password verification, approval checks, and issues JWT tokens (with HttpOnly Cookies).
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req UserCreateRequest
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") || strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		req.Username = r.FormValue("username")
		req.Password = r.FormValue("password")
	} else {
		if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			req.Username = r.FormValue("username")
			req.Password = r.FormValue("password")
			if req.Username == "" && req.Password == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Invalid JSON payload"})
				return
			}
		}
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	// 1. Check user existence in PostgreSQL
	user, err := db.Queries.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Invalid username or password"})
		return
	}

	// 2. Verify password with Bcrypt
	if !security.VerifyPassword(req.Password, user.PasswordHash) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Invalid username or password"})
		return
	}

	// 3. Check admin approval status
	if !user.IsApproved {
		w.WriteHeader(http.StatusForbidden)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Your account is pending Administrator approval"})
		return
	}

	// 4. Generate JWT Access Token
	accessToken, err := security.CreateAccessToken(user.Username, user.Role, h.Config.SecretKey, h.Config.AccessTokenExpireMinutes)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to generate authentication token"})
		return
	}

	refreshToken, err := security.CreateToken(user.Username, user.Role, security.TokenUseRefresh, h.Config.SecretKey, h.Config.RefreshTokenExpireMinutes)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to generate authentication token"})
		return
	}

	// Access token có thể được trả về JSON cho client API; refresh token thì tuyệt đối không
	// được trả trong body/header. Nó chỉ nằm trong HttpOnly cookie để JavaScript không đọc được.
	h.setTokenCookies(w, accessToken, refreshToken)

	// Touch online status in Redis
	state.TouchUserOnline(r.Context(), user.ID)

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "bearer",
		"message":      "Login successful",
		"role":         user.Role,
	})
}

// setTokenCookies đặt access token và refresh token với thuộc tính phù hợp.
//
// Refresh token dùng Path=/api/auth/refresh để browser chỉ gửi nó tới endpoint refresh;
// Authorization header không bao giờ được đọc cho refresh token.
func (h *AuthHandler) setTokenCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name: "access_token", Value: "Bearer " + accessToken, Path: "/", HttpOnly: true,
		Secure: h.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: h.Config.AccessTokenExpireMinutes * 60,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "refresh_token", Value: refreshToken, Path: "/api/auth/refresh", HttpOnly: true,
		Secure: h.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: h.Config.RefreshTokenExpireMinutes * 60,
	})
}

// Refresh cấp access token mới từ refresh_token HttpOnly cookie.
//
// Không chấp nhận body, query param hay Authorization header cho refresh token. Header đó chỉ
// dành cho access token ở AuthMiddleware; đưa refresh token vào đó sẽ không bao giờ có tác dụng.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "Refresh token missing")
		return
	}

	claims, err := security.ValidateRefreshToken(cookie.Value, h.Config.SecretKey)
	if err != nil || claims.Username == "" {
		writeError(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}

	user, err := db.Queries.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !user.IsApproved {
		writeError(w, http.StatusUnauthorized, "User is not active")
		return
	}

	accessToken, err := security.CreateAccessToken(user.Username, user.Role, h.Config.SecretKey, h.Config.AccessTokenExpireMinutes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate authentication token")
		return
	}

	// Chỉ rotate access token; refresh token vẫn được browser giữ trong HttpOnly cookie và
	// không xuất hiện trong body/header.
	http.SetCookie(w, &http.Cookie{
		Name: "access_token", Value: "Bearer " + accessToken, Path: "/", HttpOnly: true,
		Secure: h.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: h.Config.AccessTokenExpireMinutes * 60,
	})
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"access_token": accessToken,
		"token_type":   "bearer",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Thuộc tính phải trùng với cookie lúc đặt, nếu không trình duyệt coi đây là một cookie
	// khác và cookie phiên cũ vẫn nằm lại.
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.Config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	// Refresh cookie có Path hẹp hơn, nên phải xoá với đúng Path đó; xoá access_token thôi
	// vẫn để phiên dài hạn còn lại trong browser.
	http.SetCookie(w, &http.Cookie{
		Name: "refresh_token", Value: "", Path: "/api/auth/refresh", HttpOnly: true,
		Secure: h.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

// Me trả về thông tin chi tiết của người dùng đang đăng nhập dựa trên JWT Token.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := currentUser(w, r)
	if !ok {
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(UserResponse{
		ID:         user.ID,
		Username:   user.Username,
		Role:       user.Role,
		IsApproved: user.IsApproved,
		IsOnline:   true,
	})
}

// usersPageSize là trần số người dùng trả về cho trang quản trị.
//
// Cùng lý do với historyPageSize: truy vấn không có trần nghĩa là một triển khai đông người
// dùng phải tải toàn bộ bảng users cho mỗi lần mở trang.
const usersPageSize = 500

// GetUsers (Admin API) lấy danh sách tất cả người dùng trong hệ thống kèm trạng thái Online thời gian thực từ Redis.
func (h *AuthHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	users, err := db.Queries.ListUsers(r.Context(), usersPageSize)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi khi lấy danh sách user"})
		return
	}

	// Một lượt hỏi Redis cho cả bảng, không phải một lượt cho mỗi hàng: trước đây vòng lặp
	// dưới đây gọi IsUserOnline cho từng người, nên độ trễ của trang tỉ lệ thuận với số người
	// dùng và mỗi lượt lại không có hạn thời gian.
	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	online := state.OnlineUsers(r.Context(), ids)

	res := make([]UserResponse, len(users))
	for i, u := range users {
		res[i] = UserResponse{
			ID:         u.ID,
			Username:   u.Username,
			Role:       u.Role,
			IsApproved: u.IsApproved,
			IsOnline:   online[u.ID],
		}
	}
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(res)
}

// ApproveUser (Admin API) duyệt tài khoản người dùng theo user_id để họ có thể đăng nhập & sử dụng hệ thống.
func (h *AuthHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := chi.URLParam(r, "user_id")

	user, err := db.Queries.ApproveUser(r.Context(), userID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "User not found"})
		return
	}

	// Xoá bản ghi đã cache, nếu không tài khoản vừa duyệt vẫn bị chặn cho tới khi TTL hết
	// và người dùng không hiểu vì sao mình vẫn chưa vào được.
	middleware.InvalidateUser(user.Username)

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"message": "User " + user.Username + " approved!",
	})
}
