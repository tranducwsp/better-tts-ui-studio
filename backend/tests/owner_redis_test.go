package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/state"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

// redisOrSkip nối thẳng tới Redis kèm mật khẩu, hoặc bỏ qua bài.
//
// Không dùng withRedis: hàm đó đi qua InitRedis, vốn chỉ đọc cấu hình từ Config và không mang
// theo mật khẩu của stack đang chạy.
func redisOrSkip(t *testing.T) {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("chưa đặt TEST_REDIS_ADDR — bỏ qua bài tích hợp")
	}

	prev := state.RedisClient
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("không nối được Redis tại %s: %v", addr, err)
	}

	state.RedisClient = client
	t.Cleanup(func() {
		state.RedisClient = prev
		_ = client.Close()
	})
}

// TestOwnerSurvivesOnRedis khoá lại lý do trường OwnerID tồn tại.
//
// Trường này có mặt để replica khác khỏi phải join DB mỗi lần hỏi tiến độ. Nhưng nó chỉ làm
// được việc đó nếu thực sự tới được Redis: trước đây GetOrCreate ghi bản đầu TRƯỚC khi
// SetOwner kịp chạy, và marshalState không có trường này, nên mọi lượt ghi tiến độ về sau
// cũng không mang nó. Kết quả là owner_id không bao giờ xuất hiện trên Redis, và cái giá mà
// trường này sinh ra để tránh thì vẫn phải trả — im lặng, ở mọi request.
func TestOwnerSurvivesOnRedis(t *testing.T) {
	redisOrSkip(t)

	const taskID = "owner-redis-1"
	const owner = "user-owner-1"

	item := state.GlobalTaskManager.GetOrCreate(taskID)
	item.SetOwner(owner)

	// Một mốc tiến độ như worker vẫn phát giữa chừng.
	item.Notify(state.TaskUpdate{Status: "processing", Progress: 10})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	t.Cleanup(func() { state.RedisClient.Del(context.Background(), "task:"+taskID) })

	val, err := state.RedisClient.Get(ctx, "task:"+taskID).Result()
	if err != nil {
		t.Fatalf("đọc trạng thái task từ Redis: %v", err)
	}

	var got struct {
		OwnerID string `json:"owner_id"`
		Status  string `json:"status"`
	}
	if err := sonic.Unmarshal([]byte(val), &got); err != nil {
		t.Fatalf("giải mã trạng thái: %v", err)
	}

	if got.OwnerID != owner {
		t.Errorf("owner_id trên Redis = %q, muốn %q — replica khác sẽ phải hỏi DB mỗi request (payload: %s)", got.OwnerID, owner, val)
	}
	if got.Status != "processing" {
		t.Errorf("status = %q, muốn processing", got.Status)
	}
}

// TestOwnerReadableFromAnotherProcess mô phỏng replica thứ hai đọc task.
//
// Đây là đường đi thật mà ownsTask dựa vào: tiến trình web B chưa từng thấy task do tiến
// trình A tạo, nên nó nạp từ Redis. Nếu owner không nằm trong đó, ownsTask rơi xuống nhánh
// hỏi DB — vẫn đúng, nhưng đúng bằng cách trả cái giá mà trường OwnerID sinh ra để tránh.
func TestOwnerReadableFromAnotherProcess(t *testing.T) {
	redisOrSkip(t)

	const taskID = "owner-redis-2"
	const owner = "user-owner-2"

	item := state.GlobalTaskManager.GetOrCreate(taskID)
	item.SetOwner(owner)
	item.Notify(state.TaskUpdate{Status: "processing", Progress: 5})
	t.Cleanup(func() { state.RedisClient.Del(context.Background(), "task:"+taskID) })

	// Tiến trình thứ hai: TaskManager mới, RAM rỗng, cùng một Redis.
	other := state.NewTaskManager()
	fetched, ok := other.Get(taskID)
	if !ok {
		t.Fatal("replica thứ hai không nạp được task từ Redis")
	}

	gotOwner, known := fetched.Owner()
	if !known || gotOwner != owner {
		t.Errorf("replica thứ hai thấy owner = %q (known=%v), muốn %q", gotOwner, known, owner)
	}
}
