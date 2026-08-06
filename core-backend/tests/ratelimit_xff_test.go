package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"core-backend/middleware"
	"core-backend/state"
)

// countingLimiter dựng một handler đếm số request đi lọt qua hạn mức.
func countingLimiter(t *testing.T, limit int) (http.Handler, *int) {
	t.Helper()

	// Bộ đếm RAM chứ không Redis: bài này đo hành vi khoá theo IP, và một Redis còn khoá cũ
	// từ bài khác sẽ làm kết quả phụ thuộc thứ tự chạy.
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	passed := 0
	h := middleware.RateLimit(limit, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			passed++
			w.WriteHeader(http.StatusOK)
		}),
	)
	return h, &passed
}

// TestRateLimit_IgnoresSpoofedXFF là lý do TRUSTED_PROXIES tồn tại.
//
// X-Forwarded-For do client đặt được. Trước đây header này được tin vô điều kiện, nên khoá
// hạn mức chính là một giá trị người gọi tự chọn: đổi header mỗi lượt là đi qua được không
// giới hạn. Đo trước khi sửa: 200/200 request lọt qua một hạn mức 10/phút — vượt hoàn toàn,
// chứ không phải "một lớp làm chậm".
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
		t.Errorf("có %d/%d request lọt qua hạn mức %d — đổi X-Forwarded-For là vượt được", *passed, attempts, limit)
	}
}

// TestRateLimit_TrustsXFFFromProxy giữ chiều ngược lại.
//
// Khi thật sự có proxy phía trước, mọi người dùng đến từ cùng một địa chỉ TCP. Bỏ qua header
// lúc đó nghĩa là cả site dùng chung một khoá hạn mức, và người thứ mười một bị chặn vì mười
// người trước đã đăng nhập.
func TestRateLimit_TrustsXFFFromProxy(t *testing.T) {
	middleware.SetTrustedProxies([]string{"10.10.0.0/16"})
	t.Cleanup(func() { middleware.SetTrustedProxies(nil) })

	const limit = 2
	handler, passed := countingLimiter(t, limit)

	// Năm người dùng khác nhau, cùng đi qua một proxy tin cậy.
	for i := range 5 {
		req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		req.RemoteAddr = "10.10.0.3:5555"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	if *passed != 5 {
		t.Errorf("chỉ %d/5 người dùng sau proxy đi qua — hạn mức đang khoá theo địa chỉ proxy", *passed)
	}
}

// TestRateLimit_UsesLastUntrustedHop bắt trường hợp client bịa sẵn chuỗi trước khi tới proxy.
//
// Proxy nối thêm địa chỉ thật vào CUỐI chuỗi, nên phần bên trái là thứ client tự viết. Lấy
// phần tử đầu — cách làm cũ — nghĩa là vẫn đọc đúng giá trị người gọi tự chọn, chỉ khác là
// bây giờ phải đi qua proxy mới đặt được.
func TestRateLimit_UsesLastUntrustedHop(t *testing.T) {
	middleware.SetTrustedProxies([]string{"10.10.0.0/16"})
	t.Cleanup(func() { middleware.SetTrustedProxies(nil) })

	const limit = 3
	const attempts = 50
	handler, passed := countingLimiter(t, limit)

	for i := range attempts {
		req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		req.RemoteAddr = "10.10.0.3:5555"
		// Phần bên trái do người gọi bịa; "198.51.100.77" là địa chỉ thật proxy ghi vào.
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("172.16.9.%d, 198.51.100.77", i%256))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	if *passed != limit {
		t.Errorf("có %d/%d request lọt qua hạn mức %d — phần bịa sẵn trong X-Forwarded-For vẫn được tin", *passed, attempts, limit)
	}
}
