package state_test

import (
	"testing"
	"time"

	"backend/state"
)

// TestCleanup_KeepsRunningTask is an invariant the long synthesis path relies on.
//
// Previously Cleanup deleted by age regardless of status, and TTS_CLIENT_TIMEOUT_SECONDS allows
// up to an hour. A job longer than the threshold got deleted mid-flight, then the next Get call
// created a SECOND TaskItem from Redis — while synth.Run still held the pointer to the old one.
// When the object store failed and the RAM copy was the only fallback, the audio lived in copy A
// while the handler read copy B.
func TestCleanup_KeepsRunningTask(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	tm := state.NewTaskManager()
	item := tm.GetOrCreate("cleanup-running")
	item.Notify(state.TaskUpdate{Status: "processing", Progress: 40})
	item.CreatedAt = time.Now().Add(-30 * time.Minute)

	tm.Cleanup()

	if _, ok := tm.Peek("cleanup-running"); !ok {
		t.Error("running task was evicted from RAM — the synthesis process would write to a copy nobody reads")
	}
}

// TestCleanup_KeepsWatchedTask keeps tasks that still have SSE watchers.
//
// Deleting from the map while an SSE stream is open causes the next viewer to subscribe to a
// different object, and two copies coexist for the same task.
func TestCleanup_KeepsWatchedTask(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	tm := state.NewTaskManager()
	item := tm.GetOrCreate("cleanup-watched")
	item.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	item.CreatedAt = time.Now().Add(-30 * time.Minute)

	ch := item.Subscribe()
	defer item.Unsubscribe(ch)

	tm.Cleanup()

	if _, ok := tm.Peek("cleanup-watched"); !ok {
		t.Error("task with SSE watchers was evicted")
	}
}

// TestCleanup_RemovesFinishedTask holds the opposite: done and unwatched must be evicted,
// otherwise the map only grows.
func TestCleanup_RemovesFinishedTask(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	tm := state.NewTaskManager()
	item := tm.GetOrCreate("cleanup-finished")
	item.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	item.CreatedAt = time.Now().Add(-30 * time.Minute)

	tm.Cleanup()

	if _, ok := tm.Peek("cleanup-finished"); ok {
		t.Error("finished and unwatched task should have been evicted")
	}
}

// TestCleanup_RemovesAbandonedTask is the absolute ceiling.
//
// When a worker dies mid-flight, nobody sets the task to a terminal state. Without a ceiling,
// those tasks stay forever — exactly the memory leak that keeping running tasks can create.
func TestCleanup_RemovesAbandonedTask(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	tm := state.NewTaskManager()
	item := tm.GetOrCreate("cleanup-abandoned")
	item.Notify(state.TaskUpdate{Status: "processing", Progress: 40})
	item.CreatedAt = time.Now().Add(-3 * time.Hour)

	tm.Cleanup()

	if _, ok := tm.Peek("cleanup-abandoned"); ok {
		t.Error("stuck task past the age ceiling should have been evicted")
	}
}
