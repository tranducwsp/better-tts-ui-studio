package state_test

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/state"

	"github.com/redis/go-redis/v9"
)

// testRedis connects to Redis for the test, skipping when the environment is unavailable.
//
// Worker and web run in separate processes so they cannot see each other's RAM; this test plays
// both roles using two separate TaskManagers, with Redis as the only bridge between them.
//
// Run:
//
//	docker run -d --rm -p 127.0.0.1:6399:6379 redis:alpine
//	TEST_REDIS_ADDR=127.0.0.1:6399 go test ./tests/integration/state -run TestCancel -v
func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR (e.g. 127.0.0.1:6399) to run Redis integration tests")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("TEST_REDIS_PASSWORD")})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Skipf("cannot connect to Redis at %s: %v", addr, err)
	}
	return rdb
}

// useRedis attaches a temporary client to the global RedisClient and restores it after the test.
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
		t.Fatalf("cleaning up Redis task %s: %v", taskID, err)
	}
	t.Cleanup(func() {
		_ = rdb.Del(context.Background(), "task:"+taskID).Err()
	})
}

// TestCancelBeforeWorkerWatches reproduces bug C1: the user presses stop before the worker picks up the job.
// The web writes the cancel flag to Redis, then the worker in a separate process registers. The worker
// must recover the cancel flag from Redis instead of overwriting it with a new TaskItem.
func TestCancelBeforeWorkerWatches(t *testing.T) {
	rdb := testRedis(t)
	useRedis(t, rdb)

	taskID := "test-cancel-before-subscribe"
	clearTask(t, rdb, taskID)

	web := state.NewTaskManager()
	web.GetOrCreate(taskID)
	if !web.Cancel(taskID) {
		t.Fatal("web could not find the task to cancel")
	}

	worker := state.NewTaskManager()
	item := worker.GetOrCreate(taskID)
	if !item.IsCancelled() {
		t.Fatal("worker rebuilding task from Redis must see the cancel flag written by web, but IsCancelled() = false")
	}

	ctx, stop := item.WatchCancel(context.Background())
	defer stop()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("WatchCancel did not cancel even though the task was already requested to stop before it registered")
	}
}

// TestCancelDuringSubscribeGap checks that a cancel command arrives after the worker creates the task
// but before WatchCancel registers. Pub/sub does not replay messages, so WatchCancel must read Redis state.
func TestCancelDuringSubscribeGap(t *testing.T) {
	rdb := testRedis(t)
	useRedis(t, rdb)

	taskID := "test-cancel-during-gap"
	clearTask(t, rdb, taskID)

	worker := state.NewTaskManager()
	item := worker.GetOrCreate(taskID)
	if item.IsCancelled() {
		t.Fatal("newly created task must not carry a cancel flag")
	}

	web := state.NewTaskManager()
	if !web.Cancel(taskID) {
		t.Fatal("web could not find the task to cancel")
	}

	ctx, stop := item.WatchCancel(context.Background())
	defer stop()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("WatchCancel did not see the cancel command issued before it registered")
	}
	if !item.IsCancelled() {
		t.Fatal("WatchCancel detecting cancel via Redis must set the local flag")
	}
}

// TestCancelAfterWorkerWatching tests the normal path: the worker is already listening on the channel
// before the web issues the cancel command.
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
		t.Fatal("web could not find the task to cancel")
	}

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("WatchCancel did not receive the cancelled message even though it registered before")
	}
	if !item.IsCancelled() {
		t.Fatal("cancelled message via Redis must set the local flag")
	}
}
