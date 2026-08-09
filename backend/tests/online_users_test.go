package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/state"

	"github.com/redis/go-redis/v9"
)

// TestOnlineUsers_OneRoundTrip khoá lại lý do OnlineUsers tồn tại.
//
// Trang quản trị hỏi trạng thái online cho cả bảng người dùng. Trước đây nó gọi IsUserOnline
// trong vòng lặp — một round-trip Redis cho mỗi hàng — nên độ trễ của trang tỉ lệ thuận với số
// người dùng, và mỗi lượt lại không có hạn thời gian. Bài này đếm số lệnh Redis thật sự đi ra.
func TestOnlineUsers_OneRoundTrip(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("chưa đặt TEST_REDIS_ADDR — bỏ qua bài tích hợp")
	}

	var commands int
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("REDIS_PASSWORD"),
	})
	client.AddHook(countingHook{n: &commands})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("không nối được Redis: %v", err)
	}

	prev := state.RedisClient
	state.RedisClient = client
	t.Cleanup(func() {
		state.RedisClient = prev
		_ = client.Close()
	})

	// Ba người online, hai người không.
	ids := []string{"u1", "u2", "u3", "u4", "u5"}
	for _, id := range []string{"u1", "u3", "u5"} {
		state.TouchUserOnline(ctx, id)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			client.Del(context.Background(), "user:online:"+id)
		}
	})

	commands = 0
	online := state.OnlineUsers(ctx, ids)

	if commands != 1 {
		t.Errorf("OnlineUsers dùng %d lệnh Redis cho %d người — muốn 1", commands, len(ids))
	}

	for _, id := range []string{"u1", "u3", "u5"} {
		if !online[id] {
			t.Errorf("%s phải là online", id)
		}
	}
	for _, id := range []string{"u2", "u4"} {
		if online[id] {
			t.Errorf("%s không được là online", id)
		}
	}
}

// TestOnlineUsers_NoRedis trả về map rỗng thay vì panic khi chạy không có Redis.
func TestOnlineUsers_NoRedis(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	online := state.OnlineUsers(context.Background(), []string{"a", "b"})
	if len(online) != 0 {
		t.Errorf("không có Redis thì không ai online, nhận %v", online)
	}
}

// TestOnlineUsers_EmptyInput không gọi Redis khi danh sách rỗng: MGET không nhận zero key và
// sẽ trả lỗi, biến một trang không có người dùng nào thành một lỗi.
func TestOnlineUsers_EmptyInput(t *testing.T) {
	online := state.OnlineUsers(context.Background(), nil)
	if len(online) != 0 {
		t.Errorf("danh sách rỗng phải trả map rỗng, nhận %v", online)
	}
}

// countingHook đếm số lệnh Redis đi ra, để phân biệt một lượt MGET với N lượt EXISTS.
type countingHook struct{ n *int }

func (countingHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h countingHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		*h.n++
		return next(ctx, cmd)
	}
}

func (h countingHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		*h.n += len(cmds)
		return next(ctx, cmds)
	}
}
