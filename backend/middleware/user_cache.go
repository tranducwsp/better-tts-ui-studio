package middleware

import (
	"context"
	"log"
	"sync"
	"time"

	"backend/db/sqlc"
	"backend/state"

	"github.com/bytedance/sonic"
)

// userCache remembers user records for a very short time after token validation.
//
// Without it, every authenticated request is a SELECT to PostgreSQL to re-derive what is
// essentially already in the signed token. This middleware runs on the global chain, so it
// hits /health, /ready and CORS preflight as well; the task progress endpoint is also polled
// continuously by the browser while a job runs. At a few hundred requests per second, most of
// the connection pool (default 25) is spent querying the same row.
//
// The cache lives on Redis exclusively. When Redis is unavailable the cache degrades to a
// pass-through: every request queries PostgreSQL directly. There is no in-process RAM fallback
// because it would need dual-write and dual-delete logic that is easy to get wrong, and the
// benefit (a few seconds of tolerance during a Redis flicker) does not justify the complexity.
// With multiple replicas, an in-process copy also means invalidating a user on one replica
// would leave stale data on the others until the TTL expires.
type userCache struct {
	mu  sync.RWMutex
	ttl time.Duration
}

// cachedUser is the shape written to Redis.
//
// Does not carry PasswordHash: anything outside the process is another copy of sensitive
// information that needs protection, and a bcrypt hash sitting in an unencrypted Redis is a
// leak with no compensating benefit. Verified: only Login reads this field, and it reads
// directly from PostgreSQL, not through the context.
type cachedUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	IsApproved bool   `json:"is_approved"`
}

func (c cachedUser) toUser() sqlc.User {
	return sqlc.User{
		ID:         c.ID,
		Username:   c.Username,
		Role:       c.Role,
		IsApproved: c.IsApproved,
	}
}

func fromUser(u sqlc.User) cachedUser {
	return cachedUser{
		ID:         u.ID,
		Username:   u.Username,
		Role:       u.Role,
		IsApproved: u.IsApproved,
	}
}

// globalUserCache is configured once at startup via ConfigureUserCache.
var globalUserCache = &userCache{}

// userCacheKey adds a prefix so this cache's keys don't collide with other keys on the same Redis.
func userCacheKey(username string) string { return "auth:user:" + username }

// ConfigureUserCache sets the cache TTL. ttl <= 0 disables the cache entirely.
func ConfigureUserCache(ttl time.Duration) {
	globalUserCache.mu.Lock()
	globalUserCache.ttl = ttl
	globalUserCache.mu.Unlock()
}

// InvalidateUser removes a user from the cache, so the next authentication reads from the DB.
//
// Call everywhere that changes role or approval status: otherwise, a newly approved account
// would still be rejected until the TTL expires, and the user would not understand why.
func InvalidateUser(username string) {
	if rdb := state.RedisClient; rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), cacheOpTimeout)
		defer cancel()
		if err := rdb.Del(ctx, userCacheKey(username)).Err(); err != nil {
			// Worth logging: the stale record will live for the remainder of the TTL on every
			// replica, so a newly permission-changed account may still behave with the old
			// permissions for a few seconds.
			log.Printf("user cache: failed to delete %q from Redis: %v", username, err)
		}
	}
}

// cacheOpTimeout bounds the time of a cache operation.
//
// The cache sits on the path of EVERY authenticated request, so a slow Redis must not become
// system-wide latency: on timeout, treat it as a cache miss and go straight to PostgreSQL,
// slower but still correct.
const cacheOpTimeout = 100 * time.Millisecond

// get returns a valid cached record, or ok=false if missing/expired/cache is disabled.
func (c *userCache) get(username string) (sqlc.User, bool) {
	c.mu.RLock()
	ttl := c.ttl
	c.mu.RUnlock()

	if ttl <= 0 {
		return sqlc.User{}, false
	}

	if rdb := state.RedisClient; rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), cacheOpTimeout)
		defer cancel()

		raw, err := rdb.Get(ctx, userCacheKey(username)).Bytes()
		if err != nil {
			// Includes redis.Nil (key not present) — both just mean we must query the DB.
			return sqlc.User{}, false
		}
		var cu cachedUser
		if err := sonic.Unmarshal(raw, &cu); err != nil {
			return sqlc.User{}, false
		}
		return cu.toUser(), true
	}

	return sqlc.User{}, false
}

// put remembers a record in Redis.
func (c *userCache) put(username string, user sqlc.User) {
	c.mu.RLock()
	ttl := c.ttl
	c.mu.RUnlock()

	if ttl <= 0 {
		return
	}

	if rdb := state.RedisClient; rdb != nil {
		raw, err := sonic.Marshal(fromUser(user))
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), cacheOpTimeout)
		defer cancel()
		// A failed write only loses one cache speedup, never yields wrong results — the next
		// request re-reads from PostgreSQL.
		_ = rdb.Set(ctx, userCacheKey(username), raw, ttl).Err()
	}
}

// The two functions below allow cache inspection without standing up PostgreSQL — the real
// entry point is lookupUser, but it goes through db.Queries so it cannot be tested independently.

// PutUserForTest writes a record into the cache. For test use only.
func PutUserForTest(username string, user sqlc.User) {
	globalUserCache.put(username, user)
}

// GetUserForTest reads a record from the cache. For test use only.
func GetUserForTest(username string) (sqlc.User, bool) {
	return globalUserCache.get(username)
}