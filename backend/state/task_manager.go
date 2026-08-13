package state

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

type TaskManager struct {
	tasks map[string]*TaskItem
	mu    sync.RWMutex
}

var GlobalTaskManager = NewTaskManager()

func NewTaskManager() *TaskManager {
	tm := &TaskManager{
		tasks: make(map[string]*TaskItem),
	}
	go tm.startCleanupRoutine()
	return tm
}

func (tm *TaskManager) GetOrCreate(taskID string) *TaskItem {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if item, ok := tm.tasks[taskID]; ok {
		return item
	}

	item := &TaskItem{
		ID:          taskID,
		Status:      "processing",
		Progress:    0,
		CreatedAt:   time.Now(),
		subscribers: make(map[chan TaskUpdate]struct{}),
	}

	// Read previously written state from Redis before constructing a new one,
	// instead of blindly overwriting.
	//
	// The web process writes task state to Redis when it receives the request
	// (Synthesize) — and may have already written a cancel flag if the user hit
	// stop right after submitting. The worker runs in a separate process and cannot
	// see the web's RAM, so this copy is its only source. Overwriting with a "clean"
	// TaskItem (Cancel=false, Status=processing) is exactly how a cancel command
	// disappears: the worker picks up a cancelled job, stomps on the cancel:true
	// that was just saved, then runs the entire synthesis that the user already
	// stopped — the DB and UI then report "done" on top of "cancelled" that was
	// already shown.
	if RedisClient != nil {
		ctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
		val, rerr := RedisClient.Get(ctx, "task:"+taskID).Result()
		rcancel()
		if rerr == nil && val != "" {
			var remote TaskItem
			if err := sonic.Unmarshal([]byte(val), &remote); err == nil {
				item.applyRemoteState(&remote)
			}
		}
	}

	tm.tasks[taskID] = item

	// Register the key on Redis if connected.
	//
	// Via marshalState rather than marshaling the whole struct: that is the single
	// write path, so the initial state and every subsequent update carry the same
	// set of fields. Marshaling *TaskItem directly would read un-locked fields and
	// produce a payload with a different shape than every later write.
	if RedisClient != nil {
		ctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
		defer rcancel()
		if taskStateData, err := item.marshalState(); err == nil {
			_ = RedisClient.Set(ctx, "task:"+taskID, taskStateData, redisTaskTTL).Err()
		}
	}

	return item
}

// Owner returns the user who owns the task, and false if the task has no recorded owner.
func (t *TaskItem) Owner() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.OwnerID, t.OwnerID != ""
}

// SetOwner assigns an owner to the task if it does not already have one, and pushes
// to Redis if the assignment was made.
//
// Does not overwrite: task_id is sent by the client, so if one user sends a task_id
// that collides with another's, the second request must not hijack ownership of the first.
//
// Must write Redis directly: GetOrCreate already wrote the initial state before the
// caller could know the owner, so modifying only RAM would leave replicas seeing an
// ownerless task. Only writes when there is an actual change — synth.Run also calls
// this on tasks that already have an owner, and that call needs no round trip.
func (t *TaskItem) SetOwner(userID string) {
	if userID == "" {
		return
	}
	t.mu.Lock()
	changed := t.OwnerID == ""
	if changed {
		t.OwnerID = userID
	}
	t.mu.Unlock()

	if !changed || RedisClient == nil {
		return
	}

	taskStateData, err := t.marshalState()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = RedisClient.Set(ctx, "task:"+t.ID, taskStateData, redisTaskTTL).Err()
}

// applyRemoteState updates the control-plane state from a copy read from Redis.
//
// Only copies fields where another process has greater authority: progress, result,
// format. Subscribers are untouched — SSE subscribers belong to this process. OwnerID
// is copied because a task reconstructed from Redis carries the recorded owner, and
// SetOwner only assigns when this copy has none. Cancel must be followed: it is a
// stop command written by the web process, and the worker restoring a TaskItem from
// Redis must see it to know the synthesis should be cut short.
func (t *TaskItem) applyRemoteState(remote *TaskItem) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Status = remote.Status
	t.Progress = remote.Progress
	t.Error = remote.Error
	t.Cancel = remote.Cancel
	if remote.SourceFormat != "" {
		t.SourceFormat = remote.SourceFormat
	}
	if remote.OwnerID != "" && t.OwnerID == "" {
		t.OwnerID = remote.OwnerID
	}
}

// Peek returns the task in this process's RAM, without backfilling from Redis.
//
// Get loads audio from Redis into RAM when RAM is empty — correct for callers about
// to serve bytes, but wrong for callers who only want to know the format: it pulls
// the entire file into RAM to answer a few-character question, and breaks the
// direct-redirect-to-storage branch.
func (tm *TaskManager) Peek(taskID string) (*TaskItem, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	item, ok := tm.tasks[taskID]
	return item, ok
}

func (tm *TaskManager) Get(taskID string) (*TaskItem, bool) {
	// 1. Check local RAM first
	tm.mu.RLock()
	item, ok := tm.tasks[taskID]
	tm.mu.RUnlock()

	if ok {
		status, sourceFormat, _, audio := item.Snapshot()

		// The in-RAM copy may be stale: the web process creates the task on request
		// and pushes it to the queue, so its copy stays at "processing" while the
		// worker in another process has already finished. Without re-reading, the
		// status never advances, even though the work is done and audio is in storage.
		//
		// Only re-read when not yet terminal: done/error/cancelled tasks never change
		// again, and asking Redis each time just adds a round trip for a known answer.
		if RedisClient != nil && status != "done" && status != "error" && status != "cancelled" {
			rctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
			val, rerr := RedisClient.Get(rctx, "task:"+taskID).Result()
			rcancel()
			if err := rerr; err == nil && val != "" {
				var fresh TaskItem
				if err := sonic.Unmarshal([]byte(val), &fresh); err == nil {
					item.applyRemoteState(&fresh)
					status, sourceFormat, _, audio = item.Snapshot()
				}
			}
		}

		// Audio may have been produced by another replica, so backfill from Redis
		// when RAM is empty. Written via SetAudio: this field is read concurrently
		// by Notify, and direct assignment is a data race.
		if RedisClient != nil && len(audio) == 0 && sourceFormat != "" && status == "done" {
			rctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
			b, err := RedisClient.Get(rctx, "task:audio:"+taskID+":"+sourceFormat).Bytes()
			rcancel()
			if err == nil {
				item.SetAudio(b)
			}
		}
		return item, true
	}

	// 2. If RAM is empty and Redis is connected (hit on another Node), load Task from Redis
	if RedisClient != nil {
		ctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
		defer rcancel()
		val, err := RedisClient.Get(ctx, "task:"+taskID).Result()
		if err == nil && val != "" {
			var fetchedItem TaskItem
			if err := sonic.Unmarshal([]byte(val), &fetchedItem); err == nil {
				fetchedItem.subscribers = make(map[chan TaskUpdate]struct{})

				// Load Audio bytes from Redis according to the source format recorded in the Task.
				if fetchedItem.SourceFormat != "" {
					if b, err := RedisClient.Get(ctx, "task:audio:"+taskID+":"+fetchedItem.SourceFormat).Bytes(); err == nil {
						fetchedItem.Audio = b
					}
				}

				tm.mu.Lock()
				tm.tasks[taskID] = &fetchedItem
				tm.mu.Unlock()

				item := tm.tasks[taskID]
				return item, true
			}
		}
	}

	return nil, false
}

// CacheTranscoded remembers a transcoded version so later downloads don't re-run ffmpeg.
//
// Written only to disk and Redis, not kept in RAM: the transcoded copy is roughly the
// same size as the original, and keeping every format a user has ever downloaded would
// multiply resident memory by the number of formats. Disk has a periodic cleanup scanner,
// Redis has TTL — both have limits, RAM does not.
func (tm *TaskManager) CacheTranscoded(taskID, format string, data []byte) {
	if len(data) == 0 || format == "" {
		return
	}

	tm.mu.RLock()
	_, ok := tm.tasks[taskID]
	tm.mu.RUnlock()
	if !ok {
		return
	}

	if RedisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = RedisClient.Set(ctx, "task:audio:"+taskID+":"+format, data, redisTaskTTL).Err()
	}
}

// WatchCancel returns a context that is cancelled when this task is requested to stop.
//
// Must go through Redis, not just read the in-RAM flag: the user hitting stop talks
// to the web process, while the synthesis runs in the worker — two different processes
// that cannot see each other's RAM. Cancel publishes "cancelled" on channel:task:<id>,
// so the worker listens on that same channel.
//
// Check the local flag before listening: in no-Redis mode, as well as when the cancel
// arrived before the worker picked up the job, the state is already in RAM and there
// is no message to wait for.
//
// The caller must call stop to release the subscription and goroutine.

func (tm *TaskManager) Cancel(taskID string) bool {
	item, ok := tm.Get(taskID)
	if !ok {
		return false
	}

	item.RequestCancel()
	_, _, progress, _ := item.Snapshot()
	item.Notify(TaskUpdate{
		Status:   "cancelled",
		Progress: progress,
	})
	return true
}

// Cleanup evicts finished tasks that no longer have watchers.
//
// Only removes tasks in a terminal state. Previously it removed by age regardless of
// state, and TTS_CLIENT_TIMEOUT_SECONDS allows up to an hour: a job longer than this
// threshold was deleted mid-flight, then the next Get reconstructed a SECOND TaskItem
// from Redis. The synthesis process still held the pointer to the old copy, so it wrote
// audio and status into an object that no handler was still reading — when storage write
// failed and the in-RAM copy was the only fallback, the audio existed but the user got
// "Audio is not ready".
//
// Subscribers are preserved: removing from the map while an SSE stream is open causes
// the next viewer to subscribe to a different object, and two copies coexist for one task.
func (tm *TaskManager) Cleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	for id, t := range tm.tasks {
		if taskRetention > 0 && now.Sub(t.CreatedAt) <= taskRetention {
			continue
		}
		if !t.finishedAndIdle() {
			// Unfinished tasks still need a ceiling: if the worker dies mid-flight,
			// no one sets it to a terminal state, and keeping it forever is a memory
			// leak that never closes on its own. This ceiling is wider than the
			// normal threshold to avoid cutting a genuinely long-running job.
			if now.Sub(t.CreatedAt) <= taskMaxLifetime {
				continue
			}
			log.Printf("Task %s still at %q after %s — evicting from RAM", id, t.currentStatus(), taskMaxLifetime)
		}
		delete(tm.tasks, id)
	}

	// If the map has been completely cleaned, reinitialize it to release old Go map buckets for GC.
	if len(tm.tasks) == 0 {
		tm.tasks = make(map[string]*TaskItem)
	}
}

// finishedAndIdle reports whether the task has reached a terminal state and has no remaining watchers.
func (t *TaskItem) finishedAndIdle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch t.Status {
	case "done", "error", "cancelled":
	default:
		return false
	}
	return len(t.subscribers) == 0
}

// currentStatus reads the status for logging.
func (t *TaskItem) currentStatus() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Status
}

func (tm *TaskManager) startCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		tm.Cleanup()
	}
}
