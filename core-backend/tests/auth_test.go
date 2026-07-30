package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"core-backend/config"
	"core-backend/db/sqlc"
	"core-backend/handlers"
	"core-backend/middleware"
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
	for _, c := range cookies {
		if c.Name == "access_token" {
			logoutCookie = c
			break
		}
	}

	if logoutCookie == nil || logoutCookie.MaxAge != -1 {
		t.Errorf("Expected access_token cookie to be expired (-1), got: %v", logoutCookie)
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
