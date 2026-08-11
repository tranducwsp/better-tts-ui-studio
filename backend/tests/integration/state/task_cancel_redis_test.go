package state_test

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/state"

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
//	TEST_REDIS_ADDR=127.0.0.1:6399 go test ./tests/integration/state -run TestCancel -v
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

// useRedis gắn một client tạm vào RedisClient toàn cục và khôi phục sau bài test.
func useRedis(t *testing.T, rdb *redis.Client) {
	t.Helper()
	prev := state.RedisClient
	state.RedisClient = rdb
	t.Cleanup(func() {
		state.RedisClient = prev
		_ = rdb.Close()
	})
}

func clearTask(t *testing.T, rdb *redis.Client, taskID string) {
	t.Helper()
	ctx := context.Background()
	if err := rdb.Del(ctx, "task:"+taskID).Err(); err != nil {
		t.Fatalf("dọn task Redis %s: %v", taskID, err)
	}
	t.Cleanup(func() {
		_ = rdb.Del(context.Background(), "task:"+taskID).Err()
	})
}

// TestCancelBeforeWorkerWatches tái hiện lỗi C1: người dùng bấm dừng trước khi worker nhặt job.
// Web ghi cờ huỷ lên Redis, rồi worker ở tiến trình khác mới đăng ký. Worker phải phục hồi cờ
// huỷ từ Redis thay vì ghi đè nó bằng một TaskItem mới.
func TestCancelBeforeWorkerWatches(t *testing.T) {
	rdb := testRedis(t)
	useRedis(t, rdb)

	taskID := "test-cancel-before-subscribe"
	clearTask(t, rdb, taskID)

	web := state.NewTaskManager()
	web.GetOrCreate(taskID)
	if !web.Cancel(taskID) {
		t.Fatal("web không tìm thấy task để huỷ")
	}

	worker := state.NewTaskManager()
	item := worker.GetOrCreate(taskID)
	if !item.IsCancelled() {
		t.Fatal("worker dựng lại task từ Redis phải thấy cờ huỷ web đã ghi, nhưng IsCancelled() = false")
	}

	ctx, stop := item.WatchCancel(context.Background())
	defer stop()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("WatchCancel không huỷ dù task đã bị yêu cầu dừng trước khi nó đăng ký")
	}
}

// TestCancelDuringSubscribeGap kiểm tra lệnh huỷ đến sau khi worker dựng task nhưng trước khi
// WatchCancel đăng ký. Pub/sub không phát lại bản tin, nên WatchCancel phải đọc trạng thái Redis.
func TestCancelDuringSubscribeGap(t *testing.T) {
	rdb := testRedis(t)
	useRedis(t, rdb)

	taskID := "test-cancel-during-gap"
	clearTask(t, rdb, taskID)

	worker := state.NewTaskManager()
	item := worker.GetOrCreate(taskID)
	if item.IsCancelled() {
		t.Fatal("task vừa tạo không được mang cờ huỷ")
	}

	web := state.NewTaskManager()
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
		t.Fatal("WatchCancel phát hiện huỷ qua Redis phải đặt lại cờ cục bộ")
	}
}

// TestCancelAfterWorkerWatching kiểm tra đường bình thường: worker đã nghe kênh trước khi web
// phát lệnh huỷ.
func TestCancelAfterWorkerWatching(t *testing.T) {
	rdb := testRedis(t)
	useRedis(t, rdb)

	taskID := "test-cancel-after-subscribe"
	clearTask(t, rdb, taskID)

	worker := state.NewTaskManager()
	item := worker.GetOrCreate(taskID)
	ctx, stop := item.WatchCancel(context.Background())
	defer stop()

	web := state.NewTaskManager()
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
