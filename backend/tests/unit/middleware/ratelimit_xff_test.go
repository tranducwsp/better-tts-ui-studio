package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/middleware"
	"backend/state"
)

// countingLimiter builds a handler that counts how many requests pass through the limit.
func countingLimiter(t *testing.T, limit int) (http.Handler, *int) {
	t.Helper()

	// In-memory counter, not Redis: this test measures IP-based limiting behavior, and a Redis
	// with stale keys from another test would make results order-dependent.
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	passed := 0
	h := middleware.RateLimit("test", limit, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			passed++
			w.WriteHeader(http.StatusOK)
		}),
	)
	return h, &passed
}

// TestRateLimit_IgnoresSpoofedXFF is the reason TRUSTED_PROXIES exists.
//
// X-Forwarded-For can be set by the client. Previously this header was trusted unconditionally,
// so the rate-limit key was a value the caller chose: rotating the header each request bypassed
// the limit entirely. Measured before the fix: 200/200 requests passed a 10/min limit — fully
// bypassed, not just "slowed down a little."

func TestRateLimit_IgnoresSpoofedXFF(t *testing.T) {
	middleware.SetTrustedProxies(nil)

	const limit = 10
	const attempts = 200
	handler, passed := countingLimiter(t, limit)

	for i := range attempts {
		req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		req.RemoteAddr = "203.0.113.9:5555"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i%256))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	if *passed != limit {
		t.Errorf("%d/%d requests passed limit %d — rotating X-Forwarded-For bypasses the limit", *passed, attempts, limit)
	}
}

// TestRateLimit_TrustsXFFFromProxy holds the opposite direction.
//
// When there really is a proxy in front, all users arrive from the same TCP address. Ignoring
// the header then means the entire site shares one rate-limit key, and the eleventh person is
// blocked because ten people logged in before them.
func TestRateLimit_TrustsXFFFromProxy(t *testing.T) {
	middleware.SetTrustedProxies([]string{"10.10.0.0/16"})
	t.Cleanup(func() { middleware.SetTrustedProxies(nil) })

	const limit = 2
	handler, passed := countingLimiter(t, limit)

	// Five different users, all passing through the same trusted proxy.
	for i := range 5 {
		req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		req.RemoteAddr = "10.10.0.3:5555"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	if *passed != 5 {
		t.Errorf("only %d/5 users behind the proxy passed — the limit is keyed by the proxy address", *passed)
	}
}

// TestRateLimit_UsesLastUntrustedHop catches the case where the client pre-spoofs the chain
// before reaching the proxy.
//
// The proxy appends the real address to the END of the chain, so the left part is what the
// client wrote. Taking the first element — the old approach — means still reading the value
// the caller chose, except now they have to go through the proxy to set it.
func TestRateLimit_UsesLastUntrustedHop(t *testing.T) {
	middleware.SetTrustedProxies([]string{"10.10.0.0/16"})
	t.Cleanup(func() { middleware.SetTrustedProxies(nil) })

	const limit = 3
	const attempts = 50
	handler, passed := countingLimiter(t, limit)

	for i := range attempts {
		req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		req.RemoteAddr = "10.10.0.3:5555"
		// Left part is spoofed by the caller; "198.51.100.77" is the real address the proxy wrote.
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("172.16.9.%d, 198.51.100.77", i%256))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	if *passed != limit {
		t.Errorf("%d/%d requests passed limit %d — the pre-spoofed part of X-Forwarded-For is still being trusted", *passed, attempts, limit)
	}
}
