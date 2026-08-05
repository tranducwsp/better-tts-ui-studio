package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"core-backend/config"
	"core-backend/db/sqlc"
	"core-backend/middleware"
	"core-backend/state"

	"github.com/redis/go-redis/v9"
)

// Địa chỉ Redis dùng cho các bài kiểm thử tích hợp dưới đây.
//
// Không có Redis thì bỏ qua thay vì hỏng: bộ kiểm thử phải chạy được trên một máy trống, và
// nhánh RAM đã có bài riêng. Đặt TEST_REDIS_ADDR để trỏ sang chỗ khác.
func testRedisAddr() string {
	if v := strings.TrimSpace(os.Getenv("TEST_REDIS_ADDR")); v != "" {
		return v
	}
	return "127.0.0.1:56379"
}

// withRedis nối tới Redis và dọn sạch DB, hoặc bỏ qua bài kiểm thử.
func withRedis(t *testing.T) {
	t.Helper()

	prev := state.RedisClient
	state.InitRedis(&config.Config{RedisURL: testRedisAddr()})
	if state.RedisClient == nil || state.RedisClient == prev {
		state.RedisClient = prev
		t.Skipf("không có Redis tại %s — bỏ qua bài tích hợp", testRedisAddr())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := state.RedisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("không dọn được Redis: %v", err)
	}

	t.Cleanup(func() { state.RedisClient = prev })
}

// withDeadRedis trỏ client vào một cổng không ai nghe, để mô phỏng Redis sập giữa lúc chạy.
//
// Không dùng InitRedis: hàm đó ping trước và giữ nguyên client cũ khi thất bại, nên nó không
// dựng được trạng thái cần kiểm ở đây.
func withDeadRedis(t *testing.T) {
	t.Helper()
	prev := state.RedisClient
	state.RedisClient = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { state.RedisClient = prev })
}

func loginOnce(t *testing.T, h http.Handler, ip string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.RemoteAddr = ip + ":1111"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

// TestRedisRateLimit_SharedAcrossReplicas là lý do lớp này chuyển sang Redis.
//
// Mỗi lần gọi RateLimit dựng một limiter riêng, đúng như mỗi replica có bộ đếm riêng của nó.
// Với bộ đếm trong RAM, người dò mật khẩu chỉ cần được load balancer rải đều là nhân hạn mức
// lên bấy nhiêu lần — và đó là trạng thái mặc định của một deployment nhiều node.
func TestRedisRateLimit_SharedAcrossReplicas(t *testing.T) {
	withRedis(t)

	const limit = 3
	const ip = "203.0.113.99"

	for i := 1; i <= limit; i++ {
		replica := middleware.RateLimit(limit, time.Minute)(okHandler())
		if code := loginOnce(t, replica, ip); code != http.StatusOK {
			t.Fatalf("lượt %d trong hạn mức phải đi qua, got %d", i, code)
		}
	}

	other := middleware.RateLimit(limit, time.Minute)(okHandler())
	if code := loginOnce(t, other, ip); code != http.StatusTooManyRequests {
		t.Errorf("lượt vượt hạn mức phải bị chặn kể cả khi đi qua replica khác, got %d", code)
	}
}

// TestRedisRateLimit_WindowSlides xác nhận cửa sổ thật sự trượt, không phải cửa sổ cố định.
func TestRedisRateLimit_WindowSlides(t *testing.T) {
	withRedis(t)

	const ip = "198.51.100.77"
	h := middleware.RateLimit(2, 2*time.Second)(okHandler())

	if c1, c2 := loginOnce(t, h, ip), loginOnce(t, h, ip); c1 != http.StatusOK || c2 != http.StatusOK {
		t.Fatalf("hai lượt đầu phải qua, got %d %d", c1, c2)
	}
	if code := loginOnce(t, h, ip); code != http.StatusTooManyRequests {
		t.Fatalf("lượt thứ ba phải bị chặn, got %d", code)
	}

	time.Sleep(2100 * time.Millisecond)

	if code := loginOnce(t, h, ip); code != http.StatusOK {
		t.Errorf("sau khi lượt cũ rơi khỏi cửa sổ thì phải đi được, got %d", code)
	}
}

// TestRedisRateLimit_DownFallsBackNotOpen: sự cố Redis không được biến thành cửa mở.
func TestRedisRateLimit_DownFallsBackNotOpen(t *testing.T) {
	withDeadRedis(t)

	const ip = "203.0.113.5"
	h := middleware.RateLimit(2, time.Minute)(okHandler())

	if c1, c2 := loginOnce(t, h, ip), loginOnce(t, h, ip); c1 != http.StatusOK || c2 != http.StatusOK {
		t.Fatalf("hai lượt đầu phải qua, got %d %d", c1, c2)
	}
	if code := loginOnce(t, h, ip); code != http.StatusTooManyRequests {
		t.Errorf("Redis hỏng vẫn phải chặn bằng bộ đếm RAM, got %d", code)
	}
}

// TestRedisUserCache_SharedAndInvalidated khoá lại cả hai nửa của việc chuyển sang Redis:
// các replica đọc chung một bản, và Invalidate xoá cho tất cả cùng lúc.
//
// Nửa sau mới là chỗ trước đây sai hẳn chứ không chỉ kém hiệu quả: admin duyệt tài khoản ở
// replica A chỉ xoá cache của A, nên replica B vẫn từ chối người đó tới hết TTL — người dùng
// bấm lại vài lần thì lúc được lúc không, tuỳ load balancer.
func TestRedisUserCache_SharedAndInvalidated(t *testing.T) {
	withRedis(t)

	middleware.ConfigureUserCache(time.Minute)
	defer middleware.ConfigureUserCache(0)

	middleware.PutUserForTest("dave", sqlc.User{
		ID: "u9", Username: "dave", Role: "admin", IsApproved: true,
	})

	got, ok := middleware.GetUserForTest("dave")
	if !ok {
		t.Fatal("phải đọc lại được bản ghi từ Redis")
	}
	if got.ID != "u9" || got.Role != "admin" || !got.IsApproved {
		t.Errorf("bản ghi đọc về không khớp: %+v", got)
	}

	middleware.InvalidateUser("dave")
	if _, ok := middleware.GetUserForTest("dave"); ok {
		t.Error("InvalidateUser phải xoá khoá trên Redis, không chỉ trong RAM của một tiến trình")
	}
}

// TestRedisUserCache_NeverStoresPasswordHash giữ bcrypt hash nằm lại trong PostgreSQL.
//
// Redis ở đây không mã hoá và thường dùng chung cho nhiều thứ, nên đẩy hash lên đó là thêm
// một bản sao của thứ nhạy cảm nhất trong bảng users mà không đổi lại lợi ích nào — cache
// chỉ cần role và trạng thái duyệt.
func TestRedisUserCache_NeverStoresPasswordHash(t *testing.T) {
	withRedis(t)

	middleware.ConfigureUserCache(time.Minute)
	defer middleware.ConfigureUserCache(0)

	const marker = "$2a$10$UNIQUEHASHMARKER"
	middleware.PutUserForTest("frank", sqlc.User{
		ID: "u10", Username: "frank", Role: "user", IsApproved: true, PasswordHash: marker,
	})

	if got, _ := middleware.GetUserForTest("frank"); got.PasswordHash != "" {
		t.Errorf("bản ghi đọc từ cache không được mang PasswordHash, got %q", got.PasswordHash)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := state.RedisClient.Get(ctx, "auth:user:frank").Result()
	if err != nil {
		t.Fatalf("không đọc được khoá cache trên Redis: %v", err)
	}
	if strings.Contains(raw, marker) {
		t.Errorf("payload trên Redis chứa password hash: %s", raw)
	}
}

// TestRedisUserCache_DownIsMiss: Redis hỏng phải báo trượt cache, không trả dữ liệu sai.
func TestRedisUserCache_DownIsMiss(t *testing.T) {
	withDeadRedis(t)

	middleware.ConfigureUserCache(time.Minute)
	defer middleware.ConfigureUserCache(0)

	middleware.PutUserForTest("erin", sqlc.User{ID: "u11", Username: "erin"})
	if _, ok := middleware.GetUserForTest("erin"); ok {
		t.Error("Redis hỏng phải báo trượt để người gọi hỏi thẳng PostgreSQL")
	}
}
