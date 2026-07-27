package handlers

import (
	"net/http"

	"core-backend/config"
	"core-backend/db"
	"core-backend/db/sqlc"
	"core-backend/middleware"
	"core-backend/security"

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
}

// Register xử lý đăng ký tài khoản người dùng mới.
// Mặc định tài khoản mới sẽ ở trạng thái chờ duyệt (IsApproved = false).
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req UserCreateRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Vui lòng nhập đầy đủ username và password"})
		return
	}

	// 1. Kiểm tra xem username đã tồn tại chưa
	_, err := db.Queries.GetUserByUsername(r.Context(), req.Username)
	if err == nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Username already registered"})
		return
	}

	// 2. Hash mật khẩu bằng thuật toán Bcrypt
	hashedPassword, err := security.HashPassword(req.Password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi mã hóa mật khẩu"})
		return
	}

	// 3. Tạo tài khoản người dùng trong CSDL PostgreSQL qua sqlc
	_, err = db.Queries.CreateUser(r.Context(), sqlc.CreateUserParams{
		ID:           uuid.NewString(),
		Username:     req.Username,
		PasswordHash: hashedPassword,
		Role:         "user",
		IsApproved:   false,
	})

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi khi lưu tài khoản"})
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Đăng ký thành công! Vui lòng chờ Admin duyệt tài khoản."})
}

// Login xử lý đăng nhập, kiểm tra mật khẩu và thiết lập JWT Access Token qua HttpOnly Cookie.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	username := ""
	password := ""

	// Hỗ trợ nhận dữ liệu từ JSON payload hoặc Form Data
	if r.Header.Get("Content-Type") == "application/json" {
		var req UserCreateRequest
		_ = sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req)
		username = req.Username
		password = req.Password
	} else {
		_ = r.ParseForm()
		username = r.FormValue("username")
		password = r.FormValue("password")
	}

	if username == "" || password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Vui lòng nhập username và password"})
		return
	}

	// 1. Kiểm tra tài khoản có tồn tại không
	user, err := db.Queries.GetUserByUsername(r.Context(), username)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Sai tài khoản hoặc mật khẩu"})
		return
	}

	// 2. Xác thực mật khẩu Bcrypt
	if !security.VerifyPassword(password, user.PasswordHash) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Sai tài khoản hoặc mật khẩu"})
		return
	}

	// 3. Kiểm tra xem tài khoản đã được Admin phê duyệt chưa
	if !user.IsApproved {
		w.WriteHeader(http.StatusForbidden)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Tài khoản chưa được Admin duyệt. Vui lòng chờ!"})
		return
	}

	// 4. Tạo JWT Access Token
	accessToken, err := security.CreateAccessToken(user.Username, user.Role, h.Config.SecretKey, h.Config.AccessTokenExpireMinutes)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi tạo token"})
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
	})
}

// GetUsers (Admin API) lấy danh sách tất cả người dùng trong hệ thống.
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
