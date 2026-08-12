package state_test

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/state"

	"github.com/redis/go-redis/v9"
)

// TestOnlineUsers_OneRoundTrip locks in the reason OnlineUsers exists.
//
// The admin page queries online status for the entire user table. Previously it called
// IsUserOnline in a loop — one Redis round-trip per row — so page latency was proportional to
// the number of users, and each call had no time bound. This test counts the actual Redis
// commands sent.

func TestOnlineUsers_OneRoundTrip(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set — skipping integration test")
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
		t.Skipf("cannot connect to Redis: %v", err)
	}

	prev := state.RedisClient
	state.RedisClient = client
	t.Cleanup(func() {
		state.RedisClient = prev
		_ = client.Close()
	})

	// Three users online, two offline.
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
		t.Errorf("OnlineUsers used %d Redis commands for %d users — want 1", commands, len(ids))
	}

	for _, id := range []string{"u1", "u3", "u5"} {
		if !online[id] {
			t.Errorf("%s should be online", id)
		}
	}
	for _, id := range []string{"u2", "u4"} {
		if online[id] {
			t.Errorf("%s should not be online", id)
		}
	}
}

// TestOnlineUsers_NoRedis returns an empty map instead of panicking when running without Redis.
func TestOnlineUsers_NoRedis(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	online := state.OnlineUsers(context.Background(), []string{"a", "b"})
	if len(online) != 0 {
		t.Errorf("without Redis no one is online, got %v", online)
	}
}

// TestOnlineUsers_EmptyInput does not call Redis with an empty list: MGET does not accept
// zero keys and would error, turning a page with no users into an error.
func TestOnlineUsers_EmptyInput(t *testing.T) {
	online := state.OnlineUsers(context.Background(), nil)
	if len(online) != 0 {
		t.Errorf("empty list must return an empty map, got %v", online)
	}
}

// countingHook counts Redis commands sent, to distinguish a single MGET from N EXISTS calls.

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
