package handlers

import (
	"net/http"
	"strings"
	"time"

	"backend/config"
	"backend/db"
	"backend/db/sqlc"
	"backend/middleware"
	"backend/security"
	"backend/state"

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

	// Kiểm trước khi băm: bcrypt chỉ nhận 72 byte, nên password dài hơn nữa khiến
	// HashPassword dưới trả lỗi và người dùng thấy "500 Failed to hash password" cho một
	// lỗi thuộc về đầu vào. Chặn ở đây trả 400 với đúng lý do. Vị trí nằm sau kiểm rỗng để
	// hai trường hợp có hai thông điệp riêng.
	if err := security.ValidatePassword(req.Password); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
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
	//
	// Password quá 72 byte bị bcrypt từ chối so sánh; trả về 401 chung như mọi sai mật khẩu
	// thay vì để bcrypt báo lỗi riêng (trong quá khứ điều này từng ra 500). Cũng 401 như
	// VerifyPassword bên dưới sẽ trả, nhưng viết thẳng ở đây là chủ động, không tùy thuộc
	// vào cách thư viện đóng gói lỗi.
	if len([]byte(req.Password)) > security.MaxPasswordBytes {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Invalid username or password"})
		return
	}
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

	// 4. Dọn phiên quá hạn. Opportunistic tại login: bảng nhỏ, DELETE theo index, không cần
	// một tiến trình dọn riêng. Lỗi không chặn đăng nhập.
	_, _ = db.DeleteExpiredAuthSessions(r.Context())

	// 5. Tạo phiên dài hạn phía server: mỗi lần đăng nhập là một family riêng. Refresh token
	// chỉ sống bằng tuổi dòng auth_sessions này; rotation luôn giữ nguyên mốc hết hạn.
	sessExpiresAt := time.Now().Add(time.Duration(h.Config.RefreshTokenExpireMinutes) * time.Minute)
	jti := uuid.NewString()
	if err := db.CreateAuthSession(r.Context(), jti, uuid.NewString(), user.ID, sessExpiresAt, r.UserAgent(), middleware.ClientIP(r)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to create session"})
		return
	}

	// 6. Generate JWT Access Token
	accessToken, err := security.CreateAccessToken(user.Username, user.Role, h.Config.SecretKey, h.Config.AccessTokenExpireMinutes)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to generate authentication token"})
		return
	}

	refreshToken, err := security.CreateRefreshToken(user.Username, user.Role, jti, h.Config.SecretKey, sessExpiresAt)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to generate authentication token"})
		return
	}

	// Access token có thể được trả về JSON cho client API; refresh token thì tuyệt đối không
	// được trả trong body/header. Nó chỉ nằm trong HttpOnly cookie để JavaScript không đọc được.
	h.setTokenCookies(w, accessToken, refreshToken, sessExpiresAt)

	// Touch online status in Redis
	state.TouchUserOnline(r.Context(), user.ID)

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "bearer",
		"message":      "Login successful",
		"role":         user.Role,
		// Phải kèm user object, nếu không frontend gán response này làm currentUser thì header
		// không biết tên người dùng là gì. Register đã làm vậy; login thiếu là một lỗi.
		"user": UserResponse{
			ID:         user.ID,
			Username:   user.Username,
			Role:       user.Role,
			IsApproved: user.IsApproved,
			IsOnline:   true,
		},
	})
}

// setTokenCookies đặt access token và refresh token với thuộc tính phù hợp.
//
// Refresh token dùng Path=/api/auth để browser gửi nó tới đúng hai nơi cần: endpoint refresh
// (POST /api/auth/refresh) và logout (POST /api/auth/logout) — logout phải nhìn thấy token
// mới thu hồi được nó phía server. Không hẹp hơn tới /api/auth/refresh vì khi đó logout nằm
// ngoài phạm vi path của cookie và phiên không bao giờ bị chết. Authorization header không bao
// giờ được đọc cho refresh token. MaxAge tính từ mốc hết hạn TUYỆT ĐỐI của phiên: sau mỗi
// rotation, tuổi còn lại ngắn dần.
func (h *AuthHandler) setTokenCookies(w http.ResponseWriter, accessToken, refreshToken string, refreshExpiresAt time.Time) {
	maxAge := int(time.Until(refreshExpiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name: "access_token", Value: "Bearer " + accessToken, Path: "/", HttpOnly: true,
		Secure: h.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: h.Config.AccessTokenExpireMinutes * 60,
	})
	h.setRefreshCookie(w, refreshToken, maxAge)
}

// setRefreshCookie đặt một refresh cookie tại Path mới (/api/auth), hoặc xoá nó khi value rỗng.
// Tách riêng để Logout còn xoá được cookie legacy Path=/api/auth/refresh từ các phiên đăng
// nhập trước bản sửa này.
func (h *AuthHandler) setRefreshCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: "refresh_token", Value: value, Path: "/api/auth", HttpOnly: true,
		Secure: h.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: maxAge,
	})
}

// Refresh cấp access token mới từ refresh_token HttpOnly cookie, đồng thời rotate refresh token.
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
	if err != nil || claims.Username == "" || claims.JTI == "" {
		writeError(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}

	// Tra cứu dòng phiên theo jti. Token ký đúng nhưng jti không tồn tại (giả mạo, bảng đã bị
	// xoá khi phiên hết hạn) phải bị từ chối như token rác.
	sess, err := db.GetAuthSession(r.Context(), claims.JTI)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Session not found")
		return
	}

	// Token đã bị thay thế: đây là reuse. Chỉ từ chối token cũ, KHÔNG hạ family. Browser dùng
	// chung một cookie jar cho mọi tab; tab thua chỉ cần gửi lại là có cookie mới nhất — nếu
	// hạ family ở đây thì chỉ cần hai tab cùng refresh là đá mọi phiên khác ra ngoài.
	if sess.ReplacedBy.Valid {
		writeError(w, http.StatusUnauthorized, "Refresh token already used")
		return
	}
	// Phiên đã bị thu hồi chủ động khi logout: nguyên family đã chết, token này không còn
	// quyền gì nữa.
	if sess.RevokedAt.Valid {
		writeError(w, http.StatusUnauthorized, "Refresh token revoked")
		return
	}
	// Phiên hết hạn theo DB (rotation KHÔNG kéo dài phiên). Thu hồi token để ai giữ nó quay
	// lại sau khi đã trả 401 cũng không thể dùng lại được nữa.
	if sess.ExpiresAt.Time.Before(time.Now()) {
		_, _ = db.RevokeAuthSession(r.Context(), claims.JTI)
		writeError(w, http.StatusUnauthorized, "Session expired")
		return
	}

	// jti của phiên phải thuộc đúng người dùng đang yêu cầu; chống tráo refresh token giữa
	// hai tài khoản.
	user, err := db.Queries.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !user.IsApproved || user.ID != sess.UserID {
		writeError(w, http.StatusUnauthorized, "User is not active")
		return
	}

	// Rotation trong một transaction: đẻ token mới (jti mới, cùng family, cùng mốc hết hạn
	// TUYỆT ĐỐI) rồi mới đánh dấu token cũ — không bao giờ có hai token cùng hợp lệ.
	newJTI := uuid.NewString()
	if err := db.RotateRefreshSession(r.Context(), claims.JTI, sess.SessionFamilyID, user.ID, newJTI, sess.ExpiresAt.Time, r.UserAgent(), middleware.ClientIP(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to rotate session")
		return
	}

	accessToken, err := security.CreateAccessToken(user.Username, user.Role, h.Config.SecretKey, h.Config.AccessTokenExpireMinutes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate authentication token")
		return
	}
	refreshToken, err := security.CreateRefreshToken(user.Username, user.Role, newJTI, h.Config.SecretKey, sess.ExpiresAt.Time)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate authentication token")
		return
	}

	// Token mới thay cookie cũ; browser chỉ biết token mới từ đây trở đi.
	h.setTokenCookies(w, accessToken, refreshToken, sess.ExpiresAt.Time)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"access_token": accessToken,
		"token_type":   "bearer",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Thu hồi token phía server nếu cookie còn hợp lệ. Xoá cookie thôi thì phiên dài hạn vẫn
	// sống: kẻ trộm có token sẽ tiếp tục refresh được vô hạn. Không có token (hoặc đã hết hạn)
	// thì vẫn xoá cookie để đáp ứng có ý nghĩa.
	if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
		if claims, err := security.ValidateRefreshToken(cookie.Value, h.Config.SecretKey); err == nil && claims.JTI != "" {
			_, _ = db.RevokeAuthSession(r.Context(), claims.JTI)
		}
	}

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
	// Refresh cookie xoá ở CẢ hai Path: một cho cookie hiện hành (/api/auth), một cho cookie
	// của những phiên đăng nhập trước bản sửa scope (vốn Path=/api/auth/refresh). Bỏ sót path
	// cũ thì trình duyệt giữ lại cookie đó và phiên vẫn sống.
	h.setRefreshCookie(w, "", -1)
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
