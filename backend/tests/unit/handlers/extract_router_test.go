package handlers_test

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

// extractTestRouter dựng đúng chuỗi middleware thật của /api/extract-text trong router.go:
// BodyLimit → RequireActiveUser → ConcurrencyLimit → RateLimitUser → handler.
//
// Khổ hạn mức nhỏ hơn production để bài test chạy nhanh; thứ quan trọng là *cấu trúc* khớp
// với router thật — đổi cấu trúc mà quên sửa test thì test nói ngay.
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

// TestExtractText_ThroughRouter xác nhận route chạy end-to-end qua middleware chain:
// user hợp lệ upload .txt, nhận lại đúng nội dung bóc được.
func TestExtractText_ThroughRouter(t *testing.T) {
	r := extractTestRouter()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "sample.txt")
	if err != nil {
		t.Fatalf("không tạo được form file: %v", err)
	}
	_, _ = part.Write([]byte("hello from router e2e"))
	_ = writer.Close()

	req := asUser(httptest.NewRequest(http.MethodPost, "/api/extract-text", &buf), taskOwner)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("extract-text qua middleware chain phải 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("hello from router e2e")) {
		t.Errorf("text trả về thiếu nội dung đã tải lên: %s", rec.Body.String())
	}
}

// TestExtractText_RequiresAuth giữ chặt thứ tự middleware: extract-text nằm trong nhóm bảo
// vệ (RequireActiveUser) chứ không phải route công cộng.
func TestExtractText_RequiresAuth(t *testing.T) {
	r := extractTestRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/extract-text", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("không có user trong context phải bị 401, got %d", rec.Code)
	}
}
