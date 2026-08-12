package tests

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

// TestWatchCancel_CrossProcess is the reason WatchCancel must go through Redis.
//
// The person clicking stop talks to the web process; the synthesis runs in the worker. The two
// processes cannot see each other's RAM, so a local flag never reaches where it needs to. This
// test recreates exactly that setup: two separate TaskManagers, one shared Redis.
func TestWatchCancel_CrossProcess(t *testing.T) {
	redisOrSkip(t)

	const taskID = "watch-cancel-cross"
	t.Cleanup(func() { state.RedisClient.Del(context.Background(), "task:"+taskID) })

	// "Worker" process: running the job and listening for the stop command.
	worker := state.NewTaskManager()
	workerItem := worker.GetOrCreate(taskID)
	workerItem.Notify(state.TaskUpdate{Status: "processing", Progress: 10})

	ctx, stop := workerItem.WatchCancel(context.Background())
	defer stop()

	// go-redis subscribes lazily; wait one tick so it can register before the publish.
	time.Sleep(300 * time.Millisecond)

	select {
	case <-ctx.Done():
		t.Fatal("context cancelled before anyone requested stop")
	default:
	}

	// "Web" process: receives the user's stop click.
	web := state.NewTaskManager()
	if !web.Cancel(taskID) {
		t.Fatal("web process could not cancel the task")
	}

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("stop command from the web process did not reach the synthesis in the worker — GPU would still run to completion")
	}

	if !workerItem.IsCancelled() {
		t.Error("worker has not acknowledged the task was cancelled")
	}
}
