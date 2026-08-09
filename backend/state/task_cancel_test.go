package state

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// testRedis nối tới Redis dùng cho bài test, bỏ qua khi không có môi trường.
//
// Worker và web chạy ở hai tiến trình khác nhau nên không thấy RAM của nhau; test này đóng vai
// cả hai bằng hai TaskManager riêng biệt, với Redis làm cầu trung gian duy nhất.
//
// Chạy:
//
//	docker run -d --rm -p 127.0.0.1:6399:6379 redis:alpine
//	TEST_REDIS_ADDR=127.0.0.1:6399 go test ./state/ -run TestCancel -v
func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("đặt TEST_REDIS_ADDR (ví dụ 127.0.0.1:6399) để chạy test tích hợp Redis")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("TEST_REDIS_PASSWORD")})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Skipf("không nối được Redis tại %s: %v", addr, err)
	}
	return rdb
}

// useRedis gắn một client tạm vào RedisClient toàn cục và trả thao tác khôi phục.
func useRedis(t *testing.T, rdb *redis.Client) func() {
	t.Helper()
	prev := RedisClient
	RedisClient = rdb
	cleanup := func() {
		RedisClient = prev
		_ = rdb.Close()
	}
	t.Cleanup(cleanup)
	return cleanup
}

// TestCancelBeforeWorkerSubscribes tái hiện lỗi C1: người dùng bấm dừng TRƯỚC khi worker nhặt job.
//
// Web ghi cờ huỷ lên Redis khi nhận lệnh dừng, rồi worker ở tiến trình khác mới đăng ký. Trước
// bản sửa, worker gọi GetOrCreate dựng một TaskItem "sạch" (Cancel=false) và ghi đè lên chính
// trạng thái đó — lệnh huỷ biến mất và job cứ chạy trọn lượt. Bản sửa đọc trạng thái Redis
// trước khi dựng bản mới nên cờ huỷ phải đi qua được.
func TestCancelBeforeWorkerWatches(t *testing.T) {
	rdb := testRedis(t)
	useRedis(t, rdb)

	taskID := "test-cancel-before-subscribe"
	_ = rdb.Del(context.Background(), "task:"+taskID).Err()

	// Tiến trình "web": nhận yêu cầu tổng hợp rồi người dùng bấm dừng ngay.
	web := NewTaskManager()
	web.GetOrCreate(taskID)
	if !web.Cancel(taskID) {
		t.Fatal("web không tìm thấy task để huỷ")
	}

	// Tiến trình "worker": nhặt job ở tiến trình khác, không thấy RAM của web.
	worker := NewTaskManager()
	item := worker.GetOrCreate(taskID)

	if !item.IsCancelled() {
		t.Fatal("worker dựng lại task từ Redis phải thấy cờ huỷ web đã ghi, nhưng IsCancelled() = false")
	}

	// WatchCancel phải trả về context đã huỷ ngay: không có bản tin nào để chờ nữa.
	ctx, stop := item.WatchCancel(context.Background())
	defer stop()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("WatchCancel không huỷ dù task đã bị yêu cầu dừng trước khi nó đăng ký")
	}
}

// TestCancelDuringSubscribeGap là khe thứ hai của C1: lệnh huỷ tới sau khi worker đã dựng task
// nhưng trước khi WatchCancel kịp đăng ký nghe kênh.
//
// Redis pub/sub không phát lại bản tin cho người đăng ký muộn, nên nếu WatchCancel chỉ dựa vào
// bản tin thì lệnh huỷ này mất vĩnh viễn. Bản sửa đọc lại trạng thái trên Redis ngay sau khi
// subscribe — Cancel ghi trạng thái TRƯỚC khi Publish (xem Notify), nên bản ghi phải có mặt.
func TestCancelDuringSubscribeGap(t *testing.T) {
	rdb := testRedis(t)
	useRedis(t, rdb)

	taskID := "test-cancel-during-gap"
	_ = rdb.Del(context.Background(), "task:"+taskID).Err()

	// Worker dựng task trước, chưa ai huỷ.
	worker := NewTaskManager()
	item := worker.GetOrCreate(taskID)
	if item.IsCancelled() {
		t.Fatal("task vừa tạo không được mang cờ huỷ")
	}

	// Web bấm dừng đúng vào khoảng giữa: đã ghi trạng thái và publish lên kênh, nhưng worker
	// chưa hề đăng ký — bản tin đi vào hư không.
	web := NewTaskManager()
	if !web.Cancel(taskID) {
		t.Fatal("web không tìm thấy task để huỷ")
	}

	ctx, stop := item.WatchCancel(context.Background())
	defer stop()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("WatchCancel không thấy lệnh huỷ đã phát trước khi nó đăng ký")
	}

	if !item.IsCancelled() {
		t.Fatal("WatchCancel phát hiện huỷ qua Redis phải đặt lại cờ cục bộ (RequestCancel)")
	}
}

// TestCancelAfterWorkerWatching: lệnh huỷ tới khi WatchCancel đã đang nghe kênh.
//
// Đây là đường may mắn — subscription đã sẵn sàng thì publish tới được — và nó phải tiếp tục
// hoạt động như trước, để chắc chắn bản sửa không phá ngược đường đang chạy.
func TestCancelAfterWorkerWatching(t *testing.T) {
	rdb := testRedis(t)
	useRedis(t, rdb)

	taskID := "test-cancel-after-subscribe"
	_ = rdb.Del(context.Background(), "task:"+taskID).Err()

	// Worker dựng task và đăng ký nghe TRƯỚC khi có ai huỷ.
	worker := NewTaskManager()
	item := worker.GetOrCreate(taskID)
	ctx, stop := item.WatchCancel(context.Background())
	defer stop()

	web := NewTaskManager()
	if !web.Cancel(taskID) {
		t.Fatal("web không tìm thấy task để huỷ")
	}

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("WatchCancel không nhận được bản tin cancelled dù đã đăng ký trước")
	}
	if !item.IsCancelled() {
		t.Fatal("bản tin cancelled qua Redis phải đặt cờ cục bộ")
	}
}