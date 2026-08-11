package state_test

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/state"

	"github.com/redis/go-redis/v9"
)

// TestOnlineUsers_OneRoundTrip kiểm tra OnlineUsers chỉ gọi một lệnh Redis.
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
