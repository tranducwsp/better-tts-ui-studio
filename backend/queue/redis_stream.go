package queue

import (
	"context"
	"errors"
	"time"

	"backend/state"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

// streamKey is the stream that holds all pending synthesis requests.
const streamKey = "tts:jobs"

// consumerGroup groups all workers so Redis distributes jobs without duplicates.
const consumerGroup = "tts-workers"

// maxStreamLen is the cap on jobs held in the stream.
//
// This stream is NOT durable: Redis runs without AOF/RDB, so everything in here is lost on
// Redis restart. That is intentional — a chunk takes only a few seconds to re-synthesize, and
// the UI already auto-retries three times per chunk. The trade-off is not having to maintain
// another durable store, and not having to keep job state in sync across two places.
//
// This cap exists only so an abnormal burst doesn't consume all of Redis's RAM; MAXLEN is
// approximate so Redis evicts in blocks, which is cheaper than exact eviction.
const maxStreamLen = 10000

// ErrNoRedis means the queue is unavailable because Redis is missing.
//
// The web caller uses it to fall back to in-process synthesis: a deployment without Redis can
// still synthesize, it just can't separate workers.
var ErrNoRedis = errors.New("queue: Redis not configured")

// Job is a synthesis request waiting to be processed.
//
// It carries everything a worker needs to run without asking anyone else: the worker may be in
// a different process, on a different machine, and cannot see the RAM of the process that
// received the request.
type Job struct {
	TaskID  string   `json:"task_id"`
	UserID  string   `json:"user_id"`
	Text    string   `json:"text"`
	Voice   string   `json:"voice"`
	Engine  string   `json:"engine"`
	Speed   float64  `json:"speed"`
	Pitch   *float64 `json:"pitch,omitempty"`
	Emotion *string  `json:"emotion,omitempty"`
}

// Enqueue pushes a job into the queue.
func Enqueue(ctx context.Context, job Job) error {
	rdb := state.RedisClient
	if rdb == nil {
		return ErrNoRedis
	}

	payload, err := sonic.Marshal(job)
	if err != nil {
		return err
	}

	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: maxStreamLen,
		Approx: true,
		Values: map[string]any{"job": payload},
	}).Err()
}

// EnsureGroup creates the consumer group if it does not exist.
//
// MKSTREAM so a worker that starts before the web process can still create the group on a
// stream that doesn't exist yet. BUSYGROUP means the group already exists — common when
// running multiple workers, and not an error.
func EnsureGroup(ctx context.Context) error {
	rdb := state.RedisClient
	if rdb == nil {
		return ErrNoRedis
	}

	err := rdb.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// Consume reads jobs until ctx is cancelled, calling handle for each job.
//
// Each process gets a distinct consumer name so Redis can tell workers in the same group apart.
//
// Ack immediately after handle returns, even if it reported an error: a broken job has already
// been written with error status and the UI will retry automatically, so keeping it in the
// pending list only creates garbage no one reads. For the same reason, there is no XAUTOCLAIM
// here: if a worker dies mid-job, the job is lost, and that is the chosen behavior.
func Consume(ctx context.Context, consumer string, handle func(context.Context, Job)) error {
	rdb := state.RedisClient
	if rdb == nil {
		return ErrNoRedis
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		// Block with a timeout rather than indefinitely: we need to loop back periodically
		// to notice that ctx has been cancelled.
		res, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumer,
			Streams:  []string{streamKey, ">"},
			Count:    1,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) || ctx.Err() != nil {
				continue // timeout, no job — normal
			}
			// Redis flickered: wait a beat then retry instead of hot-looping.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}

		for _, stream := range res {
			for _, msg := range stream.Messages {
				raw, _ := msg.Values["job"].(string)

				var job Job
				if err := sonic.Unmarshal([]byte(raw), &job); err != nil {
					// Can't decode means no one can process it; ack so it doesn't
					// linger forever.
					_ = rdb.XAck(ctx, streamKey, consumerGroup, msg.ID).Err()
					continue
				}

				handle(ctx, job)
				_ = rdb.XAck(ctx, streamKey, consumerGroup, msg.ID).Err()
			}
		}
	}
}
