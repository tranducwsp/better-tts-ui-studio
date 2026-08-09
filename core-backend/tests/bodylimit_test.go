package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"core-backend/middleware"
)

// bodyTestHandler là một handler đọc hết body rồi trả 200 — giống đúng các handler JSON
// (auth, synthesize, history) vốn decode tới EOF. Ghép với middleware.BodyLimit để quan sát
// cách body quá trần phản ảnh qua lỗi đọc.
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

// TestBodyLimit_BlocksOversizedJSON xác nhận body JSON quá 2 MiB bị phản ánh như lỗi đọc
// (chứ không dựng hết vào RAM rồi xử lý như trước C3).
func TestBodyLimit_BlocksOversizedJSON(t *testing.T) {
	h := middleware.BodyLimit(bodyTestHandler())

	big := strings.Repeat("a", 3<<20) // 3 MiB > jsonBodyLimit 2 MiB
	req := httptest.NewRequest(http.MethodPost, "/api/synthesize/x", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("body quá trần phải phản ánh 413, got %d", rec.Code)
	}
}

// TestBodyLimit_allowsMultipart: upload giọng nói là multipart, qua middleware phải nguyên
// vẹn — handler upload tự đặt trần MAX_UPLOAD_SIZE_MB riêng.
func TestBodyLimit_allowsMultipart(t *testing.T) {
	h := middleware.BodyLimit(bodyTestHandler())

	big := strings.Repeat("b", 5<<20) // 5 MiB — vượt trần JSON nhưng multipart bị bỏ qua
	req := httptest.NewRequest(http.MethodPost, "/api/clone/upload", strings.NewReader(big))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("multipart không được bị BodyLimit chặn, got %d", rec.Code)
	}
}

// TestBodyLimit_allowsNormalBody: body dưới trần đi qua không suy chuyển.
func TestBodyLimit_allowsNormalBody(t *testing.T) {
	h := middleware.BodyLimit(bodyTestHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"a","password":"b"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("body nhỏ phải đi qua, got %d", rec.Code)
	}
}