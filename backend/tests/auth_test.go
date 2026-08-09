package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend/config"
	"backend/db/sqlc"
	"backend/handlers"
	"backend/middleware"
)

func TestAuth_Logout(t *testing.T) {
	cfg := &config.Config{SecretKey: "test-secret", AccessTokenExpireMinutes: 60}
	h := handlers.NewAuthHandler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	var logoutCookie *http.Cookie
	var refreshLogoutCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case "access_token":
			logoutCookie = c
		case "refresh_token":
			refreshLogoutCookie = c
		}
	}

	if logoutCookie == nil || logoutCookie.MaxAge != -1 {
		t.Errorf("Expected access_token cookie to be expired (-1), got: %v", logoutCookie)
	}
	if refreshLogoutCookie == nil || refreshLogoutCookie.MaxAge != -1 || refreshLogoutCookie.Path != "/api/auth/refresh" {
		t.Errorf("Expected refresh_token cookie to expire at its narrow path, got: %v", refreshLogoutCookie)
	}
}

func TestAuth_Register_EmptyPayload(t *testing.T) {
	cfg := &config.Config{SecretKey: "test-secret"}
	h := handlers.NewAuthHandler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/register", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty registration payload, got %d", rec.Code)
	}
}

// TestAuth_Register_PasswordLength chặn cú 500 cũ: password quá 72 byte từng làm
// HashPassword lỗi và trả "500 Failed to hash password" cho một lỗi của đầu vào. Validation
// chạy trước khi chạm DB (GetUserByUsername ở sau), nên test không cần database.
func TestAuth_Register_PasswordLength(t *testing.T) {
	cfg := &config.Config{SecretKey: "test-secret"}
	h := handlers.NewAuthHandler(cfg)

	cases := []struct {
		name     string
		password string
		wantLen  int
	}{
		{"short", "1234567", 7},
		{"long-over-72-bytes", strings.Repeat("a", 80), 80},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			payload := fmt.Sprintf(`{"username":"u-%d","password":%q}`, c.wantLen, c.password)
			req := httptest.NewRequest(http.MethodPost, "/api/register", strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.Register(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("password dài %d byte phải bị chặn 400, got %d", c.wantLen, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "72 byte") && !strings.Contains(rec.Body.String(), "8 ký tự") {
				t.Errorf("thông điệp 400 phải nói lý do, got: %s", rec.Body.String())
			}
		})
	}
}

func TestAuth_Me_Unauthenticated(t *testing.T) {
	cfg := &config.Config{SecretKey: "test-secret"}
	h := handlers.NewAuthHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated /me, got %d", rec.Code)
	}
}

func TestAuth_Me_Authenticated(t *testing.T) {
	cfg := &config.Config{SecretKey: "test-secret"}
	h := handlers.NewAuthHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)

	// Inject current user into request context
	user := &sqlc.User{
		ID:         "test-user-id-123",
		Username:   "testuser",
		Role:       "user",
		IsApproved: true,
	}
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var res handlers.UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to unmarshal user response: %v", err)
	}

	if res.Username != "testuser" || res.ID != "test-user-id-123" {
		t.Errorf("Unexpected user response content: %v", res)
	}
}
