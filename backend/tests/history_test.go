package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/db/sqlc"
	"backend/handlers"
	"backend/middleware"

	"github.com/go-chi/chi/v5"
)

func TestHistory_GetUserHistory_Unauthenticated(t *testing.T) {
	h := handlers.NewHistoryHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	rec := httptest.NewRecorder()

	h.GetUserHistory(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated history request, got %d", rec.Code)
	}
}

func TestHistory_GetJobDetail_Unauthenticated(t *testing.T) {
	h := handlers.NewHistoryHandler()

	r := chi.NewRouter()
	r.Get("/history/{job_id}", h.GetJobDetail)

	req := httptest.NewRequest(http.MethodGet, "/history/some-job-id", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated job detail, got %d", rec.Code)
	}
}

func TestHistory_InitJob_Unauthenticated(t *testing.T) {
	h := handlers.NewHistoryHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/init", bytes.NewReader([]byte(`{"job_id":"job-1"}`)))
	rec := httptest.NewRecorder()

	h.InitJob(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated init job, got %d", rec.Code)
	}
}

func TestHistory_InitJob_InvalidPayload(t *testing.T) {
	h := handlers.NewHistoryHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/init", bytes.NewReader([]byte(`{}`)))
	user := &sqlc.User{ID: "test-user-123", Username: "testuser", IsApproved: true}
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.InitJob(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty job_id payload, got %d", rec.Code)
	}
}

// TestHistory_GetUserHistory_BeforeNotTimestamp: con trỏ phân trang gửi `before` phải là một
// RFC3339. Giá trị không phân tích được là lỗi phía client chứ không phải máy chủ — trả 400
// trước khi chạm DB (vì vậy test này chạy được cả khi không có Postgres).
func TestHistory_GetUserHistory_BeforeNotTimestamp(t *testing.T) {
	h := handlers.NewHistoryHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/history?before=not-a-time&before_id=job-9", nil)
	user := &sqlc.User{ID: "test-user-123", Username: "testuser", IsApproved: true}
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.GetUserHistory(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for malformed before cursor, got %d: %s", rec.Code, rec.Body.String())
	}
}
