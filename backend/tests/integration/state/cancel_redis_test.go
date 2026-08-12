package state_test

import (
	"context"
	"testing"
	"time"

	"backend/state"
)

// TestWatchCancel_CrossProcess verifies cancel across two processes via Redis.
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