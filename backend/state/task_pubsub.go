package state

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

func (t *TaskItem) Subscribe() chan TaskUpdate {
	ch := make(chan TaskUpdate, 10)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.subscribers == nil {
		t.subscribers = make(map[chan TaskUpdate]struct{})
	}
	t.subscribers[ch] = struct{}{}

	// One Redis subscription per task, not per viewer: two tabs open on the same
	// task subscribe to the same channel, so duplicating the subscription only
	// makes Redis send the same payload twice while we re-distribute in-process.
	if RedisClient != nil && t.pubsubCancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		pubsub := RedisClient.Subscribe(ctx, "channel:task:"+t.ID)
		t.pubsubGeneration++
		generation := t.pubsubGeneration
		t.pubsubCancel = func() {
			cancel()
			_ = pubsub.Close()
		}
		go t.forwardRedis(ctx, pubsub, generation)
	}

	return ch
}

// forwardRedis relays messages from Redis to this task's local subscribers.
//
// The loop ends when the pubsub is closed — which is exactly what Unsubscribe does
// when the last viewer leaves. Previously this function ran with context.Background()
// and no one closed the pubsub, so every SSE open spawned a goroutine and a Redis
// subscription that lived forever: the browser auto-reconnects after every network
// drop, and the subscription count on Redis only grew.
func (t *TaskItem) forwardRedis(ctx context.Context, pubsub *redis.PubSub, generation uint64) {
	redisCh := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, open := <-redisCh:
			if !open {
				return
			}
			var update TaskUpdate
			if err := sonic.Unmarshal([]byte(msg.Payload), &update); err != nil {
				continue
			}
			t.fanoutIfCurrent(update, generation)
		}
	}
}

// fanoutIfCurrent takes the subscriber snapshot under the same lock as the generation
// check. If we only checked generation then called fanout, the old goroutine could see
// itself as valid, get unsubscribed, then fanout right as the new subscription was
// created — the new subscriber would receive the same message twice.
func (t *TaskItem) fanoutIfCurrent(update TaskUpdate, generation uint64) {
	t.mu.Lock()
	if t.pubsubGeneration != generation || t.pubsubCancel == nil {
		t.mu.Unlock()
		return
	}
	for ch := range t.subscribers {
		select {
		case ch <- update:
		default:
		}
	}
	t.mu.Unlock()
}

// fanout sends the update to every local subscriber, skipping any that are blocked.
//
// The channel has a buffer of 10 and the default branch drops messages: a slow-reading
// browser must not stall the synthesis process behind it.
func (t *TaskItem) fanout(update TaskUpdate) {
	t.mu.Lock()
	subscribers := make([]chan TaskUpdate, 0, len(t.subscribers))
	for ch := range t.subscribers {
		subscribers = append(subscribers, ch)
	}
	t.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- update:
		default:
		}
	}
}

func (t *TaskItem) Unsubscribe(ch chan TaskUpdate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.subscribers != nil {
		delete(t.subscribers, ch)
	}

	// When the last viewer leaves, close the subscription; otherwise the relay
	// goroutine remains alone with a Redis entry that no one reads.
	if len(t.subscribers) == 0 && t.pubsubCancel != nil {
		cancel := t.pubsubCancel
		t.pubsubCancel = nil
		t.pubsubGeneration++ // invalidates any final message already buffered by the old forwarder
		cancel()
	}
}

func (t *TaskItem) Notify(update TaskUpdate) {
	t.mu.Lock()
	t.Status = update.Status
	t.Progress = update.Progress
	if update.Error != "" {
		t.Error = update.Error
	}
	t.mu.Unlock()

	// When Redis is off, fan out directly in-process. When Redis is on, the shared
	// subscription delivers the message even to subscribers on the same node — if we
	// fan out both ways, this node's own Publish comes back through Redis and fans
	// out a second time, causing SSE to receive duplicate events.
	if RedisClient == nil {
		t.fanout(update)
	}

	// If Redis is active, Publish to other Nodes and save state for them to read.
	//
	// A single pipeline instead of four sequential round trips: this function runs
	// on every progress milestone, so four network waits here sit directly in the
	// synthesis process's path.
	//
	// Audio is NO longer included here. Previously task state was marshaled wholesale
	// including the Audio field, so every progress milestone pushed the entire audio
	// segment again — a 3 MB chunk times ten milestones is 30 MB of repeated traffic
	// to convey a single percentage number. Audio bytes have their own key, written
	// once when available (see CacheAudio).
	if RedisClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := sonic.Marshal(update)
	if err != nil {
		return
	}
	taskStateData, err := t.marshalState()
	if err != nil {
		return
	}

	pipe := RedisClient.Pipeline()
	// Write state BEFORE Publish, intentionally: the worker's WatchCancel re-reads
	// the task:<id> key right after subscribing, and Cancel relies on this ordering
	// to close the gap where a Publish message is lost before the worker can subscribe.
	// If the write is before the read, the worker sees the cancel flag; if the read
	// runs before the write, then Publish arrives after, reaching the subscription
	// that is now ready. Reversing the order, one execution could slip between "Set
	// not yet done" and "Publish already went through while no one was listening" —
	// exactly the gap being closed.
	pipe.Set(ctx, "task:"+t.ID, taskStateData, redisTaskTTL)
	pipe.Publish(ctx, "channel:task:"+t.ID, data)
	_, _ = pipe.Exec(ctx)
}

// CacheAudio stores audio bytes on Redis under a separate format-specific key.
//
// Intentionally separated from Notify: audio appears exactly once when the chunk is
// done, while Notify runs on every progress milestone. Merging them would cause the
// same payload to be written again at every milestone.
