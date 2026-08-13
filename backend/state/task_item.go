package state

import (
	"context"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

type TaskUpdate struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Error    string `json:"error,omitempty"`
}

type TaskItem struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`

	// OwnerID is the user who created this task, used for access control.
	//
	// Stored here instead of joining DB on every poll: status polling and SSE are
	// the two hottest paths for a running job, so a query per progress check is a
	// cost not worth paying. Empty means unknown — a task reconstructed from Redis
	// after a restart does not carry this field, and the access checker must query DB.
	OwnerID string `json:"owner_id,omitempty"`

	// Audio is the data returned by the Engine, in the declared SourceFormat. Previously
	// this field was named AudioWAV and every consumer treated it as WAV — while Edge TTS
	// mode returns MP3, so the downloaded file had a .wav extension with MP3 content inside.
	//
	// json:"-" because task state is marshaled to Redis on every progress milestone,
	// while audio bytes live under a separate task:audio:<id>:<format> key. Keeping them
	// here would mean pushing the entire audio segment on every progress update.
	Audio        []byte `json:"-"`
	SourceFormat string `json:"source_format,omitempty"`

	// Transcoded versions, cached by format so ffmpeg does not re-run on every download.
	Transcoded map[string][]byte `json:"-"`

	Cancel      bool      `json:"cancel"`
	CreatedAt   time.Time `json:"created_at"`
	Error       string    `json:"error,omitempty"`
	subscribers map[chan TaskUpdate]struct{}
	mu          sync.Mutex

	// pubsubCancel closes this task's shared Redis subscription. Nil means no one
	// is watching, or the last watcher has left.
	pubsubCancel     func()
	pubsubGeneration uint64
}

func (t *TaskItem) CacheAudio(ctx context.Context, format string, data []byte) {
	if RedisClient == nil || format == "" || len(data) == 0 {
		return
	}
	_ = RedisClient.Set(ctx, "task:audio:"+t.ID+":"+format, data, redisTaskTTL).Err()
}

// The three functions below exist because TaskItem is read by request goroutines
// while the synthesis process is writing: touching the fields directly is a data
// race that -race will flag.

// SetSourceFormat records the format the Engine just returned.
func (t *TaskItem) SetSourceFormat(format string) {
	t.mu.Lock()
	t.SourceFormat = format
	t.mu.Unlock()
}

// SetAudio holds audio bytes in RAM, used when writing to disk is not possible.
func (t *TaskItem) SetAudio(data []byte) {
	t.mu.Lock()
	t.Audio = data
	t.mu.Unlock()
}

// ReleaseAudio frees the in-RAM copy after audio has been safely persisted to disk.
func (t *TaskItem) ReleaseAudio() {
	t.mu.Lock()
	t.Audio = nil
	t.Transcoded = nil
	t.mu.Unlock()
}

func (t *TaskItem) marshalState() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Serialize only the small control-plane state. Audio and transcodes live in their own
	// Redis keys and on disk; including them here would make every progress update grow with
	// the audio payload and would also require copying it under the lock.
	// OwnerID is present here because Notify is the only write path that runs after the owner
	// is assigned: GetOrCreate writes the initial state before the handler can call SetOwner.
	// Without it, every progress write would erase the owner from Redis, and any replica
	// reading the task back would have to query DB — exactly the cost this field exists to avoid.
	return sonic.Marshal(struct {
		ID           string    `json:"id"`
		Status       string    `json:"status"`
		Progress     int       `json:"progress"`
		OwnerID      string    `json:"owner_id,omitempty"`
		SourceFormat string    `json:"source_format,omitempty"`
		Cancel       bool      `json:"cancel"`
		CreatedAt    time.Time `json:"created_at"`
		Error        string    `json:"error,omitempty"`
	}{
		ID:           t.ID,
		Status:       t.Status,
		Progress:     t.Progress,
		OwnerID:      t.OwnerID,
		SourceFormat: t.SourceFormat,
		Cancel:       t.Cancel,
		CreatedAt:    t.CreatedAt,
		Error:        t.Error,
	})
}

// Snapshot reads the fields the handler needs, under a single lock hold.
func (t *TaskItem) Snapshot() (status, sourceFormat string, progress int, audio []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Status, t.SourceFormat, t.Progress, t.Audio
}

// LastError returns the most recent error message, empty if the task has not failed.
func (t *TaskItem) LastError() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Error
}

// RequestCancel marks the task for cancellation. The synthesis process reads this
// flag between steps.
func (t *TaskItem) RequestCancel() {
	t.mu.Lock()
	t.Cancel = true
	t.mu.Unlock()
}

// IsCancelled reports whether the task has been requested to stop.
func (t *TaskItem) IsCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Cancel
}

func (t *TaskItem) WatchCancel(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	if t.IsCancelled() {
		cancel()
		return ctx, cancel
	}

	if RedisClient == nil {
		// No Redis means a single process: RequestCancel sets the flag in this same
		// RAM, so polling is sufficient and no extra infrastructure is needed.
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if t.IsCancelled() {
						cancel()
						return
					}
				}
			}
		}()
		return ctx, cancel
	}

	pubsub := RedisClient.Subscribe(ctx, "channel:task:"+t.ID)

	// A cancel command arriving between "check local flag" and "start listening on
	// channel" is lost forever: Publish only delivers to existing subscribers, with
	// no replay buffer. Close this gap by re-reading Redis state right after subscribe.
	//
	// The ordering guarantees one of the two paths always catches the cancel: Cancel
	// writes state (Set) BEFORE Publish (see Notify). If the cancel runs before this
	// read, the record is already on Redis for us to see; if it runs after, then
	// Publish also arrives after, and the subscription has already been confirmed on
	// the server side by the time the SUBSCRIBE command is processed (go-redis reads
	// the reply before the function returns) — so the message will reach the newly
	// ready subscription. No single execution falls through both gaps at once.
	rctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
	val, rerr := RedisClient.Get(rctx, "task:"+t.ID).Result()
	rcancel()
	if rerr == nil && val != "" {
		var remote TaskItem
		if sonic.Unmarshal([]byte(val), &remote) == nil && (remote.Cancel || remote.Status == "cancelled") {
			t.RequestCancel()
			cancel()
			_ = pubsub.Close()
			return ctx, cancel
		}
	}

	go func() {
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, open := <-ch:
				if !open {
					return
				}
				var update TaskUpdate
				if err := sonic.Unmarshal([]byte(msg.Payload), &update); err != nil {
					continue
				}
				if update.Status == "cancelled" {
					t.RequestCancel()
					cancel()
					return
				}
			}
		}
	}()

	return ctx, func() {
		cancel()
	}
}
