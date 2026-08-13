package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"backend/config"
	"backend/db/sqlc"
	"backend/middleware"
	"backend/state"

	"github.com/redis/go-redis/v9"
)

// Redis address used for the integration tests below.
//
// Without Redis, skip instead of failing: the test suite must run on a bare machine, and the
// in-memory branch has its own tests. Set TEST_REDIS_ADDR to point elsewhere.
func testRedisAddr() string {
	if v := strings.TrimSpace(os.Getenv("TEST_REDIS_ADDR")); v != "" {
		return v
	}
	return "127.0.0.1:56379"
}

// withRedis connects to Redis and flushes the DB, or skips the test.
func withRedis(t *testing.T) {
	t.Helper()

	prev := state.RedisClient
	state.InitRedis(&config.Config{RedisURL: testRedisAddr()})
	if state.RedisClient == nil || state.RedisClient == prev {
		state.RedisClient = prev
		t.Skipf("no Redis at %s — skipping integration test", testRedisAddr())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := state.RedisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("failed to flush Redis: %v", err)
	}

	t.Cleanup(func() { state.RedisClient = prev })
}

// withDeadRedis points the client at a port nobody is listening on, to simulate Redis going
// down mid-run.
//
// Not using InitRedis: that function pings first and keeps the old client on failure, so it
// cannot construct the state this test needs.
func withDeadRedis(t *testing.T) {
	t.Helper()
	prev := state.RedisClient
	state.RedisClient = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { state.RedisClient = prev })
}

func loginOnce(t *testing.T, h http.Handler, ip string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req.RemoteAddr = ip + ":1111"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

// TestRedisRateLimit_SharedAcrossReplicas is the reason this layer moved to Redis.
//
// Each RateLimit call builds a separate limiter, just as each replica has its own counters.
// With in-memory counters, a password guesser only needs the load balancer to spread them
// evenly to multiply the limit by that many times — and that is the default state of a
// multi-node deployment.

func TestRedisRateLimit_SharedAcrossReplicas(t *testing.T) {
	withRedis(t)

	const limit = 3
	const ip = "203.0.113.99"

	for i := 1; i <= limit; i++ {
		replica := middleware.RateLimit("test", limit, time.Minute)(okHandler())
		if code := loginOnce(t, replica, ip); code != http.StatusOK {
			t.Fatalf("request %d within limit must pass, got %d", i, code)
		}
	}

	other := middleware.RateLimit("test", limit, time.Minute)(okHandler())
	if code := loginOnce(t, other, ip); code != http.StatusTooManyRequests {
		t.Errorf("request exceeding limit must be blocked even from a different replica, got %d", code)
	}
}

// TestRedisRateLimit_WindowSlides confirms the window truly slides, not a fixed window.
func TestRedisRateLimit_WindowSlides(t *testing.T) {
	withRedis(t)

	const ip = "198.51.100.77"
	h := middleware.RateLimit("test", 2, 2*time.Second)(okHandler())

	if c1, c2 := loginOnce(t, h, ip), loginOnce(t, h, ip); c1 != http.StatusOK || c2 != http.StatusOK {
		t.Fatalf("first two requests must pass, got %d %d", c1, c2)
	}
	if code := loginOnce(t, h, ip); code != http.StatusTooManyRequests {
		t.Fatalf("third request must be blocked, got %d", code)
	}

	time.Sleep(2100 * time.Millisecond)

	if code := loginOnce(t, h, ip); code != http.StatusOK {
		t.Errorf("after old requests fall out of the window, must pass, got %d", code)
	}
}

// TestRedisRateLimit_DownFallsBackNotOpen: a Redis outage must not become an open door.
func TestRedisRateLimit_DownFallsBackNotOpen(t *testing.T) {
	withDeadRedis(t)

	const ip = "203.0.113.5"
	h := middleware.RateLimit("test", 2, time.Minute)(okHandler())

	if c1, c2 := loginOnce(t, h, ip), loginOnce(t, h, ip); c1 != http.StatusOK || c2 != http.StatusOK {
		t.Fatalf("first two requests must pass, got %d %d", c1, c2)
	}
	if code := loginOnce(t, h, ip); code != http.StatusTooManyRequests {
		t.Errorf("Redis down must still block via in-memory counter, got %d", code)
	}
}

// TestRedisUserCache_SharedAndInvalidated locks in both halves of the move to Redis:
// replicas read the same record, and Invalidate removes it for all at once.
//
// The second half is where it was previously outright wrong, not just inefficient: admin
// approving an account on replica A only cleared A's cache, so replica B still rejected that
// user until the TTL expired — the user would retry a few times and get randomly accepted or
// rejected, depending on the load balancer.

func TestRedisUserCache_SharedAndInvalidated(t *testing.T) {
	withRedis(t)

	middleware.ConfigureUserCache(time.Minute)
	defer middleware.ConfigureUserCache(0)

	middleware.PutUserForTest("dave", sqlc.User{
		ID: "u9", Username: "dave", Role: "admin", IsApproved: true,
	})

	got, ok := middleware.GetUserForTest("dave")
	if !ok {
		t.Fatal("must be able to read back the record from Redis")
	}
	if got.ID != "u9" || got.Role != "admin" || !got.IsApproved {
		t.Errorf("record read back does not match: %+v", got)
	}

	middleware.InvalidateUser("dave")
	if _, ok := middleware.GetUserForTest("dave"); ok {
		t.Error("InvalidateUser must delete the key on Redis, not just in one process's RAM")
	}
}

// TestRedisUserCache_NeverStoresPasswordHash keeps the bcrypt hash in PostgreSQL.
//
// Redis here is not encrypted and is often shared for many purposes, so pushing the hash there
// adds a copy of the most sensitive column in the users table for zero benefit — the cache only
// needs role and approval status.
func TestRedisUserCache_NeverStoresPasswordHash(t *testing.T) {
	withRedis(t)

	middleware.ConfigureUserCache(time.Minute)
	defer middleware.ConfigureUserCache(0)

	const marker = "$2a$10$UNIQUEHASHMARKER"
	middleware.PutUserForTest("frank", sqlc.User{
		ID: "u10", Username: "frank", Role: "user", IsApproved: true, PasswordHash: marker,
	})

	if got, _ := middleware.GetUserForTest("frank"); got.PasswordHash != "" {
		t.Errorf("record read from cache must not carry PasswordHash, got %q", got.PasswordHash)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := state.RedisClient.Get(ctx, "auth:user:frank").Result()
	if err != nil {
		t.Fatalf("cannot read cache key from Redis: %v", err)
	}
	if strings.Contains(raw, marker) {
		t.Errorf("payload on Redis contains password hash: %s", raw)
	}
}

// TestRedisUserCache_DownIsMiss: Redis down must report a cache miss, not return bad data.
func TestRedisUserCache_DownIsMiss(t *testing.T) {
	withDeadRedis(t)

	middleware.ConfigureUserCache(time.Minute)
	defer middleware.ConfigureUserCache(0)

	middleware.PutUserForTest("erin", sqlc.User{ID: "u11", Username: "erin"})
	if _, ok := middleware.GetUserForTest("erin"); ok {
		t.Error("Redis down must report a miss so the caller queries PostgreSQL directly")
	}
}
