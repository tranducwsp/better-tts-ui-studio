package state

import (
	"context"
	"log"
	"strings"
	"time"

	"backend/config"

	"github.com/redis/go-redis/v9"
)

var (
	// RedisClient is the connection to the Redis Server
	RedisClient *redis.Client
)

// redisTaskTTL is the time-to-live for task state and audio bytes on Redis.
const redisTaskTTL = 1 * time.Hour

// taskRetention is the minimum age before a finished task is evicted from RAM.
// Can be customized via the TASK_MEMORY_RETENTION_SECONDS env var (set to 0 to disable the In-Memory Cache).
var taskRetention = 10 * time.Minute

// ConfigureTaskRetention updates the task retention time in RAM from env config.
func ConfigureTaskRetention(d time.Duration) {
	taskRetention = d
	if d == 0 {
		log.Println("⚡ In-Memory Task Cache DISABLED (TASK_MEMORY_RETENTION_SECONDS=0): Completed tasks will be freed from RAM immediately after processing.")
	} else {
		log.Printf("🧠 In-Memory Task Cache retention configured: %v", d)
	}
}

// taskMaxLifetime is the absolute ceiling for a task in RAM, even if it has not finished.
const taskMaxLifetime = 2 * time.Hour

// taskRedisTimeout caps the time for a single read/write of task state on Redis.
const taskRedisTimeout = 100 * time.Millisecond

// InitRedis initializes the Redis Client connection from env config.
func InitRedis(cfg *config.Config) {
	ConfigureTaskRetention(time.Duration(cfg.TaskMemoryRetentionSeconds) * time.Second)

	if cfg.RedisURL == "" {
		log.Println("RedisURL is not configured, TaskManager using In-Memory Mode.")
		return
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisURL,
		Password: cfg.RedisPassword,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Cannot connect to Redis server (%v). Falling back to In-Memory Mode.", err)
		return
	}

	RedisClient = rdb
	log.Printf("Connected to Redis successfully at %s! System is now Stateless Multi-Node Ready.", cfg.RedisURL)
}

// TouchUserOnline refreshes the user's Online status in Redis with a 60-second TTL.
func TouchUserOnline(ctx context.Context, userID string) {
	if RedisClient != nil && userID != "" {
		_ = RedisClient.Set(ctx, "user:online:"+userID, "1", 60*time.Second).Err()
	}
}

// IsUserOnline checks whether a user is Online based on the Redis Key.
func IsUserOnline(ctx context.Context, userID string) bool {
	if RedisClient != nil && userID != "" {
		val, err := RedisClient.Exists(ctx, "user:online:"+userID).Result()
		return err == nil && val > 0
	}
	return false
}

// OnlineUsers reports which users in the list are Online, in a SINGLE round trip.
//
// Separated from IsUserOnline because the admin page queries the entire user table:
// calling that function in a loop is one round-trip per row, and page latency scales
// with the number of users — most noticeable when Redis is on a different machine.
//
// MGET instead of multiple EXISTS: one command, one network wait. Missing keys return
// nil, so "has a value" means "is online".
//
// Time-bound like every other Redis read on the request path: online status is
// decorative information, not worth holding up a request when Redis is slow.
func OnlineUsers(ctx context.Context, userIDs []string) map[string]bool {
	online := make(map[string]bool, len(userIDs))
	if RedisClient == nil || len(userIDs) == 0 {
		return online
	}

	keys := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id != "" {
			keys = append(keys, "user:online:"+id)
		}
	}
	if len(keys) == 0 {
		return online
	}

	opCtx, cancel := context.WithTimeout(ctx, taskRedisTimeout)
	defer cancel()

	vals, err := RedisClient.MGet(opCtx, keys...).Result()
	if err != nil {
		return online
	}

	for i, v := range vals {
		if v != nil {
			online[strings.TrimPrefix(keys[i], "user:online:")] = true
		}
	}
	return online
}
