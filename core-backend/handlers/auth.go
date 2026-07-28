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
				_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Dữ liệu JSON không hợp lệ"})
				return
			}
		}
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Username và Password không được để trống"})
		return
	}

	// 1. Kiểm tra tài khoản đã tồn tại chưa bằng sqlc
	_, err := db.Queries.GetUserByUsername(r.Context(), req.Username)
	if err == nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Tên đăng nhập đã tồn tại"})
		return
	}

	// 2. Băm mật khẩu an toàn với Bcrypt
	hashedPassword, err := security.HashPassword(req.Password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi khi xử lý mật khẩu"})
		return
	}

	// 3. Tạo record User trong PostgreSQL
	userID := uuid.NewString()
	user, err := db.Queries.CreateUser(r.Context(), sqlc.CreateUserParams{
		ID:           userID,
		Username:     req.Username,
		PasswordHash: hashedPassword,
		Role:         "user",
		IsApproved:   false, // Yêu cầu Admin phê duyệt thủ công
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi khi tạo tài khoản"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Đăng ký thành công. Vui lòng chờ Quản trị viên phê duyệt tài khoản.",
		"user": UserResponse{
			ID:         user.ID,
			Username:   user.Username,
			Role:       user.Role,
			IsApproved: user.IsApproved,
			IsOnline:   false,
		},
	})
}

// Login xử lý đăng nhập, xác thực mật khẩu, kiểm tra trạng thái duyệt và cấp phát JWT Token (kèm HttpOnly Cookie).
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
				_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Dữ liệu JSON không hợp lệ"})
				return
			}
		}
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	// 1. Kiểm tra sự tồn tại của User trong PostgreSQL
	user, err := db.Queries.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Sai tên đăng nhập hoặc mật khẩu"})
		return
	}

	// 2. Kiểm tra mật khẩu băm Bcrypt
	if !security.VerifyPassword(req.Password, user.PasswordHash) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Sai tên đăng nhập hoặc mật khẩu"})
		return
	}

	// 3. Kiểm tra trạng thái duyệt tài khoản của Admin
	if !user.IsApproved {
		w.WriteHeader(http.StatusForbidden)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Tài khoản của bạn chưa được Quản trị viên phê duyệt"})
		return
	}

	// 4. Tạo mã JWT Access Token
	accessToken, err := security.CreateAccessToken(user.Username, user.Role, h.Config.SecretKey, h.Config.AccessTokenExpireMinutes)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi khi tạo Token xác thực"})
		return
	}

	// 5. Gửi Cookie bảo mật HttpOnly về cho Browser Client
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "Bearer " + accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   h.Config.AccessTokenExpireMinutes * 60,
	})

	// Cập nhật trạng thái Online lên Redis
	state.TouchUserOnline(r.Context(), user.ID)

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "bearer",
		"message":      "Đăng nhập thành công",
		"role":         user.Role,
	})
}

// Logout xử lý đăng xuất bằng cách xoá Cookie access_token trên trình duyệt.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Đã đăng xuất"})
}

// Me trả về thông tin chi tiết của người dùng đang đăng nhập dựa trên JWT Token.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
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

// GetUsers (Admin API) lấy danh sách tất cả người dùng trong hệ thống kèm trạng thái Online thời gian thực từ Redis.
func (h *AuthHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	users, err := db.Queries.ListUsers(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi khi lấy danh sách user"})
		return
	}

	res := make([]UserResponse, len(users))
	for i, u := range users {
		res[i] = UserResponse{
			ID:         u.ID,
			Username:   u.Username,
			Role:       u.Role,
			IsApproved: u.IsApproved,
			IsOnline:   state.IsUserOnline(r.Context(), u.ID),
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

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"message": "User " + user.Username + " approved!",
	})
}
