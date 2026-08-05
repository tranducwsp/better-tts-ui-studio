package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"core-backend/middleware"
)

// TestRateLimit_BlocksBurst xác nhận lượt thứ (limit+1) trong cùng cửa sổ bị chặn.
//
// Đây là lớp duy nhất ngăn dò mật khẩu trên /login, nên nó đáng có một bài test: một
// off-by-one ở đây là im lặng, và chỉ thấy được khi có người đã dò xong.
func TestRateLimit_BlocksBurst(t *testing.T) {
	const limit = 3
	handler := middleware.RateLimit(limit, time.Minute)(
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
			t.Fatalf("Lượt %d trong hạn mức phải đi qua, got %d", i, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Lượt vượt hạn mức phải bị chặn với 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Errorf("Phản hồi 429 nên nói khi nào thử lại được")
	}
}

// TestRateLimit_PerIP xác nhận hạn mức tính riêng từng IP, để một người dùng dồn hết lượt
// không khoá cửa đăng nhập của mọi người khác.
func TestRateLimit_PerIP(t *testing.T) {
	const limit = 2
	handler := middleware.RateLimit(limit, time.Minute)(
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
		t.Errorf("IP khác phải còn nguyên hạn mức, got %d", rec.Code)
	}
}
