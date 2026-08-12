package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/db/sqlc"
	"backend/middleware"
)

// TestRateLimit_BlocksBurst confirms the (limit+1)-th request within the same window is
// blocked.
//
// This is the only layer preventing password guessing on /login, so it deserves a test: an
// off-by-one here is silent and would only be noticed after someone has already finished
// guessing.
func TestRateLimit_BlocksBurst(t *testing.T) {
	const limit = 3
	handler := middleware.RateLimit("test", limit, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	for i := 1; i <= limit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		req.RemoteAddr = "203.0.113.7:5555"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Request %d within limit must pass, got %d", i, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Request exceeding limit must be blocked with 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Errorf("429 response should indicate when to retry")
	}
}

// TestRateLimit_PerIP confirms the limit is per-IP, so one user exhausting their quota
// does not lock the login door for everyone else.
func TestRateLimit_PerIP(t *testing.T) {
	const limit = 2
	handler := middleware.RateLimit("test", limit, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	for i := 0; i < limit+1; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		req.RemoteAddr = "198.51.100.1:4444"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	other := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	other.RemoteAddr = "198.51.100.2:4444"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, other)

	if rec.Code != http.StatusOK {
		t.Errorf("Different IP must still have its full quota, got %d", rec.Code)
	}
}

// TestRateLimitUser_PerUser confirms the limit for /extract-text is keyed by authenticated
// user, not by IP: two users behind the same NAT (same RemoteAddr) have separate budgets, so
// one user hammering the endpoint does not consume the other's quota.
func TestRateLimitUser_PerUser(t *testing.T) {
	const limit = 2
	handler := middleware.RateLimitUser("extract-test", limit, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	serve := func(userID, ip string) int {
		ctx := context.WithValue(context.Background(), middleware.UserContextKey, &sqlc.User{ID: userID})
		req := httptest.NewRequest(http.MethodPost, "/api/extract-text", nil).WithContext(ctx)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// // user-a exhausts their own budget (limit requests), then the (limit+1)-th is blocked.
	for i := 0; i < limit; i++ {
		if got := serve("user-a", "203.0.113.7:5555"); got != http.StatusOK {
			t.Fatalf("user-a request %d must pass, got %d", i, got)
		}
	}
	if got := serve("user-a", "203.0.113.7:5555"); got != http.StatusTooManyRequests {
		t.Errorf("user-a exceeding limit must be blocked with 429, got %d", got)
	}

	// // user-b has not made any calls, same IP — separate budget so still passes all requests.
	for i := 0; i < limit; i++ {
		if got := serve("user-b", "203.0.113.7:5555"); got != http.StatusOK {
			t.Fatalf("user-b request %d must have its own budget, got %d", i, got)
		}
	}
}

// TestRateLimitUser_FallsBackToIP when unauthenticated: no user in context means fall back
// to IP-based limiting, so anonymous requests do not have an unlimited budget.
func TestRateLimitUser_FallsBackToIP(t *testing.T) {
	const limit = 2
	handler := middleware.RateLimitUser("extract-test", limit, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	for i := 0; i < limit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/extract-text", nil)
		req.RemoteAddr = "198.51.100.9:4444"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/extract-text", nil)
	req.RemoteAddr = "198.51.100.9:4444"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("anonymous user exceeding limit must be blocked with 429, got %d", rec.Code)
	}
}
