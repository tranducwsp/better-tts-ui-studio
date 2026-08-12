package state_test

import (
	"context"
	"testing"
	"time"

	"backend/state"
)

// TestWatchCancel_FiresOnLocalFlag is the non-Redis branch: a single process, the flag
// lives in the same RAM.
func TestWatchCancel_FiresOnLocalFlag(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	item := state.GlobalTaskManager.GetOrCreate("watch-cancel-local")

	ctx, stop := item.WatchCancel(context.Background())
	defer stop()

	select {
	case <-ctx.Done():
		t.Fatal("context cancelled before anyone requested stop")
	default:
	}

	item.RequestCancel()

	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("RequestCancel failed to cancel the context — the Engine call would run to completion despite cancellation")
	}
}

// TestWatchCancel_AlreadyCancelled catches the case where the stop command arrives before the worker picks up the job.
func TestWatchCancel_AlreadyCancelled(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	item := state.GlobalTaskManager.GetOrCreate("watch-cancel-early")
	item.RequestCancel()

	ctx, stop := item.WatchCancel(context.Background())
	defer stop()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("task was already cancelled but context is still alive")
	}
}
