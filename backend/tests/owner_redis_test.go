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

// redisOrSkip connects directly to Redis with password, or skips the test.
//
// Not using withRedis: that function goes through InitRedis, which only reads config from
// Config and does not carry the running stack's password.
func redisOrSkip(t *testing.T) {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set — skipping integration test")
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
		t.Skipf("cannot connect to Redis at %s: %v", addr, err)
	}

	state.RedisClient = client
	t.Cleanup(func() {
		state.RedisClient = prev
		_ = client.Close()
	})
}

// TestOwnerSurvivesOnRedis locks in the reason the OwnerID field exists.
//
// This field exists so other replicas don't have to join the DB on every progress query. But
// it only works if it actually reaches Redis: previously GetOrCreate wrote the initial copy
// BEFORE SetOwner ran, and marshalState did not include this field, so every subsequent
// progress write also did not carry it. The result was that owner_id never appeared on Redis,
// and the cost this field was created to avoid was still paid — silently, on every request.
func TestOwnerSurvivesOnRedis(t *testing.T) {
	redisOrSkip(t)

	const taskID = "owner-redis-1"
	const owner = "user-owner-1"

	item := state.GlobalTaskManager.GetOrCreate(taskID)
	item.SetOwner(owner)

	// A progress update as the worker would emit mid-flight.
	item.Notify(state.TaskUpdate{Status: "processing", Progress: 10})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	t.Cleanup(func() { state.RedisClient.Del(context.Background(), "task:"+taskID) })

	val, err := state.RedisClient.Get(ctx, "task:"+taskID).Result()
	if err != nil {
		t.Fatalf("read task state from Redis: %v", err)
	}

	var got struct {
		OwnerID string `json:"owner_id"`
		Status  string `json:"status"`
	}
	if err := sonic.Unmarshal([]byte(val), &got); err != nil {
		t.Fatalf("decode state: %v", err)
	}

	if got.OwnerID != owner {
		t.Errorf("owner_id on Redis = %q, want %q — other replicas would have to query DB on every request (payload: %s)", got.OwnerID, owner, val)
	}
	if got.Status != "processing" {
		t.Errorf("status = %q, want processing", got.Status)
	}
}

// TestOwnerReadableFromAnotherProcess simulates a second replica reading the task.
//
// This is the real path that ownsTask relies on: web process B has never seen the task created
// by process A, so it loads it from Redis. If the owner is not in there, ownsTask falls back to
// the DB query branch — still correct, but correct at the cost this field was created to avoid.
func TestOwnerReadableFromAnotherProcess(t *testing.T) {
	redisOrSkip(t)

	const taskID = "owner-redis-2"
	const owner = "user-owner-2"

	item := state.GlobalTaskManager.GetOrCreate(taskID)
	item.SetOwner(owner)
	item.Notify(state.TaskUpdate{Status: "processing", Progress: 5})
	t.Cleanup(func() { state.RedisClient.Del(context.Background(), "task:"+taskID) })

	// Second process: fresh TaskManager, empty RAM, same Redis.
	other := state.NewTaskManager()
	fetched, ok := other.Get(taskID)
	if !ok {
		t.Fatal("second replica could not load the task from Redis")
	}

	gotOwner, known := fetched.Owner()
	if !known || gotOwner != owner {
		t.Errorf("second replica sees owner = %q (known=%v), want %q", gotOwner, known, owner)
	}
}
