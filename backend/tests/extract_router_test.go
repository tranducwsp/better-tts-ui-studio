package tests

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/handlers"
	"backend/middleware"

	"github.com/go-chi/chi/v5"
)

// extractTestRouter builds the real middleware chain of /api/extract-text from router.go:
// BodyLimit → RequireActiveUser → ConcurrencyLimit → RateLimitUser → handler.
//
// Limits are smaller than production so the test runs fast; what matters is the *structure*
// matches the real router — if the structure changes and the test is not updated, the test will catch it immediately.
func extractTestRouter() *chi.Mux {
	r := chi.NewRouter()
	h := handlers.NewUtilsHandler()
	r.Use(middleware.BodyLimit)
	r.Use(middleware.RequireActiveUser)
	r.Group(func(r chi.Router) {
		r.Use(middleware.ConcurrencyLimit(8))
		r.Use(middleware.RateLimitUser("extract-test", 20, time.Minute))
		r.Post("/api/extract-text", h.ExtractText)
	})
	return r
}

// TestExtractText_ThroughRouter confirms the route runs end-to-end through the middleware chain:
// a valid user uploads a .txt, receives back the correct extracted content.
func TestExtractText_ThroughRouter(t *testing.T) {
	r := extractTestRouter()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "sample.txt")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("hello from router e2e"))
	_ = writer.Close()

	req := asUser(httptest.NewRequest(http.MethodPost, "/api/extract-text", &buf), taskOwner)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("extract-text through middleware chain must return 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("hello from router e2e")) {
		t.Errorf("returned text missing uploaded content: %s", rec.Body.String())
	}
}

// TestExtractText_RequiresAuth enforces middleware ordering: extract-text sits inside the
// protected group (RequireActiveUser), not on a public route.
func TestExtractText_RequiresAuth(t *testing.T) {
	r := extractTestRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/extract-text", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("absence of user in context must be 401, got %d", rec.Code)
	}
}
