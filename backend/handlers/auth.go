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

// AuthHandler handles Authentication and User Management APIs.
type AuthHandler struct {
	Config *config.Config
}

// NewAuthHandler initializes a new AuthHandler with system configuration.
func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{Config: cfg}
}

// UserCreateRequest is the request payload structure for registration or login.
type UserCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UserResponse is the user data structure returned to the client (does not contain PasswordHash).
type UserResponse struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	IsApproved bool   `json:"is_approved"`
	IsOnline   bool   `json:"is_online"`
}

// Register handles new account registration. New accounts default to IsApproved = false (pending Admin approval).
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

<<<<<<< HEAD
		// bcrypt chỉ nhận 72 byte, kiểm trước để trả 400 thay vì 500.
		if err := security.ValidatePassword(req.Password); err != nil {
=======
	// Check before hashing: bcrypt only accepts 72 bytes, so a longer password would cause
	// HashPassword below to return an error and the user would see "500 Failed to hash password"
	// for an input-level error. Block here and return 400 with the correct reason. Positioned
	// after the empty check so the two cases have distinct messages.
	if err := security.ValidatePassword(req.Password); err != nil {
>>>>>>> worktree-agent-a6f4d5609ff61827b
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
	// Passwords over 72 bytes are rejected by bcrypt comparison; return a generic 401 like any
	// wrong password instead of letting bcrypt report its own error (in the past this used to
	// return 500). Same 401 that VerifyPassword below would return, but written here proactively
	// to avoid depending on how the library wraps errors.
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

	// 4. Clean up expired sessions. Opportunistic at login: small table, DELETE by index, no need
	// for a dedicated cleanup process. Errors do not block login.
	_, _ = db.DeleteExpiredAuthSessions(r.Context())

	// 5. Create a long-lived server-side session: each login is its own family. Refresh token
	// lives only as long as this auth_sessions row; rotation always preserves the absolute expiry.
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

	// The access token can be returned in JSON for API clients; the refresh token must never
	// be returned in body/header. It only lives in an HttpOnly cookie so JavaScript cannot read it.
	h.setTokenCookies(w, accessToken, refreshToken, sessExpiresAt)

	// Touch online status in Redis
	state.TouchUserOnline(r.Context(), user.ID)

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "bearer",
		"message":      "Login successful",
		"role":         user.Role,
		// Must include the user object, otherwise the frontend assigns this response as currentUser
		// and the header has no way to know the username. Register already does this; login missing
		// it was a bug.
		"user": UserResponse{
			ID:         user.ID,
			Username:   user.Username,
			Role:       user.Role,
			IsApproved: user.IsApproved,
			IsOnline:   true,
		},
	})
}

// setTokenCookies sets the access token and refresh token with appropriate attributes.
//
// Refresh token uses Path=/api/auth so the browser sends it to exactly the two places that need
// it: the refresh endpoint (POST /api/auth/refresh) and logout (POST /api/auth/logout) — logout
// must see the token in order to revoke it server-side. Not narrower to /api/auth/refresh because
// then logout would fall outside the cookie's path scope and the session would never die. The
// Authorization header is never read for refresh tokens. MaxAge is calculated from the ABSOLUTE
// session expiry: after each rotation, the remaining lifetime gets shorter.
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

// setRefreshCookie sets a refresh cookie at the new Path (/api/auth), or clears it when value is empty.
// Separated so that Logout can also clear the legacy Path=/api/auth/refresh cookie from sessions
// created before this change.
func (h *AuthHandler) setRefreshCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: "refresh_token", Value: value, Path: "/api/auth", HttpOnly: true,
		Secure: h.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: maxAge,
	})
}

// Refresh issues a new access token from the refresh_token HttpOnly cookie, and rotates the refresh token.
//
// Does not accept body, query param, or Authorization header for the refresh token. That header is
// only for the access token in AuthMiddleware; putting a refresh token there will never have any effect.
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

	// Look up the session row by jti. A correctly signed token whose jti does not exist (forged,
	// or the row was deleted when the session expired) must be rejected like a garbage token.
	sess, err := db.GetAuthSession(r.Context(), claims.JTI)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Session not found")
		return
	}

	// Token has been replaced: this is reuse. Only reject the old token, do NOT invalidate the
	// family. The browser shares a single cookie jar across all tabs; a losing tab just needs to
	// re-send and it will get the latest cookie — if we invalidate the family here, two tabs
	// refreshing at the same time would kick out every other session.
	if sess.ReplacedBy.Valid {
		writeError(w, http.StatusUnauthorized, "Refresh token already used")
		return
	}
	// Session was actively revoked on logout: the entire family is dead, this token has no more
	// rights.
	if sess.RevokedAt.Valid {
		writeError(w, http.StatusUnauthorized, "Refresh token revoked")
		return
	}
	// Session expired per DB (rotation does NOT extend the session). Revoke the token so that
	// anyone holding it who comes back after receiving 401 cannot reuse it either.
	if sess.ExpiresAt.Time.Before(time.Now()) {
		_, _ = db.RevokeAuthSession(r.Context(), claims.JTI)
		writeError(w, http.StatusUnauthorized, "Session expired")
		return
	}

	// The session's jti must belong to the requesting user; prevents swapping refresh tokens
	// between two accounts.
	user, err := db.Queries.GetUserByUsername(r.Context(), claims.Username)
	if err != nil || !user.IsApproved || user.ID != sess.UserID {
		writeError(w, http.StatusUnauthorized, "User is not active")
		return
	}

	// Rotation in a single transaction: create the new token (new jti, same family, same ABSOLUTE
	// expiry) and only then mark the old token — there are never two valid tokens at once.
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

	// The new token replaces the old cookie; from here on the browser only knows the new token.
	h.setTokenCookies(w, accessToken, refreshToken, sess.ExpiresAt.Time)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"access_token": accessToken,
		"token_type":   "bearer",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Revoke the token server-side if the cookie is still valid. Just deleting the cookie would
	// leave the long-lived session alive: a thief holding the token could keep refreshing
	// indefinitely. If there is no token (or it has expired), still delete the cookie so the
	// response is meaningful.
	if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
		if claims, err := security.ValidateRefreshToken(cookie.Value, h.Config.SecretKey); err == nil && claims.JTI != "" {
			_, _ = db.RevokeAuthSession(r.Context(), claims.JTI)
		}
	}

	// Attributes must match the cookie at set time, otherwise the browser treats this as a
	// different cookie and the old session cookie remains.
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.Config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	// Refresh cookie is cleared on BOTH paths: one for the current cookie (/api/auth), one for
	// sessions created before the scope change (which had Path=/api/auth/refresh). Missing the
	// old path means the browser keeps that cookie and the session stays alive.
	h.setRefreshCookie(w, "", -1)
	http.SetCookie(w, &http.Cookie{
		Name: "refresh_token", Value: "", Path: "/api/auth/refresh", HttpOnly: true,
		Secure: h.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

// Me returns the currently logged-in user's details based on the JWT Token.
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

<<<<<<< HEAD
// usersPageSize là trần số người dùng trả về cho trang quản trị.
// Truy vấn không có trần nghĩa là một triển khai đông người
// phải tải toàn bộ bảng users cho mỗi lần mở trang.
=======
// usersPageSize is the cap on the number of users returned for the admin page.
//
// Same reason as historyPageSize: an un-capped query means a large deployment must load the
// entire users table every time the page is opened.
>>>>>>> worktree-agent-a6f4d5609ff61827b
const usersPageSize = 500

// GetUsers (Admin API) returns the list of all users in the system with real-time Online status from Redis.
func (h *AuthHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	users, err := db.Queries.ListUsers(r.Context(), usersPageSize)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to retrieve user list"})
		return
	}

	// One Redis query for the entire table, not one per row: previously the loop below called
	// IsUserOnline for each user, so page latency was proportional to the number of users and
	// each call had no time bound.
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

// ApproveUser (Admin API) approves a user account by user_id so they can log in and use the system.
func (h *AuthHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := chi.URLParam(r, "user_id")

	user, err := db.Queries.ApproveUser(r.Context(), userID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "User not found"})
		return
	}

	// Delete the cached record, otherwise the newly approved account is still blocked until the
	// TTL expires and the user does not understand why they still cannot log in.
	middleware.InvalidateUser(user.Username)

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"message": "User " + user.Username + " approved!",
	})
}
