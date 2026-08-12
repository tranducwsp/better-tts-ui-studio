package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend/middleware"
)

// bodyTestHandler is a handler that reads the body to completion and returns 200 — exactly
// like the JSON handlers (auth, synthesize, history) that decode to EOF. Paired with
// middleware.BodyLimit to observe how an oversized body surfaces through read errors.
func bodyTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dst strings.Builder
		if _, err := io.Copy(&dst, r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

// TestBodyLimit_BlocksOversizedJSON confirms a JSON body exceeding 2 MiB surfaces as a read
// error (rather than being fully buffered in RAM and processed, as before C3).
func TestBodyLimit_BlocksOversizedJSON(t *testing.T) {
	h := middleware.BodyLimit(bodyTestHandler())

	big := strings.Repeat("a", 3<<20) // 3 MiB > jsonBodyLimit 2 MiB
	req := httptest.NewRequest(http.MethodPost, "/api/synthesize/x", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body must surface as 413, got %d", rec.Code)
	}
}

// TestBodyLimit_allowsMultipart: voice uploads are multipart and must pass through the
// middleware intact — the upload handler sets its own MAX_UPLOAD_SIZE_MB ceiling.
func TestBodyLimit_allowsMultipart(t *testing.T) {
	h := middleware.BodyLimit(bodyTestHandler())

	big := strings.Repeat("b", 5<<20) // 5 MiB — exceeds JSON limit but multipart is exempted
	req := httptest.NewRequest(http.MethodPost, "/api/clone/upload", strings.NewReader(big))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("multipart must not be blocked by BodyLimit, got %d", rec.Code)
	}
}

// TestBodyLimit_allowsNormalBody: body under the limit passes through unchanged.
func TestBodyLimit_allowsNormalBody(t *testing.T) {
	h := middleware.BodyLimit(bodyTestHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"a","password":"b"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("small body must pass through, got %d", rec.Code)
	}
}
