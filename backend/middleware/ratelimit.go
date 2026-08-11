package middleware

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"backend/db/sqlc"
	"backend/state"

	"github.com/redis/go-redis/v9"
)

// RateLimit caps the number of requests per IP within a sliding time window.
//
// scope separates Redis keys between route groups: register+login share a single budget
// (password guessing / spam account creation), while refresh must have its OWN budget — it is
// called by the frontend on every page load, even when the user is not logged in, so merging
// it with login would let a single user exhaust the budget by refreshing the page a few times
// and lock themselves out of login. The Redis key does not distinguish which limiter instance
// wrote it, so two groups with the same scope writing to the same key means one budget.
//
// Placed on /login and /register because those two routes are where a loop has value to an
// outsider: unthrottled login is free password guessing, and unthrottled register both spams
// the users table and forces the server to run a bcrypt round on every call — bcrypt
// DefaultCost costs tens of ms of CPU, so the password protection layer itself becomes a
// lever for grinding the machine to a halt.
//
// Sliding window, not fixed window: with a fixed window, a caller who dumps all requests at
// the end of one window and the start of the next gets double the limit in the overlap moment.
//
// The counter lives on Redis when available, in-process RAM otherwise. With the in-RAM
// counter, the actual limit is limit * number of replicas: each process counts independently,
// so a password guesser only needs to be spread evenly by the load balancer to get that many
// more attempts — and that is the default state of a multi-node deployment, not a rare case.
func RateLimit(scope string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return rateLimitBy(func(r *http.Request) string { return "ip:" + ClientIP(r) }, scope, limit, window)
}

// RateLimitUser is like RateLimit but keys by the authenticated user instead of IP.
//
// Used for heavy routes (like /extract-text): a legitimate user can still abuse the system by
// keeping a call cycle; keying by user ID counts each person correctly, without someone under
// the same NAT eating the entire shared budget.
func RateLimitUser(scope string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return rateLimitBy(func(r *http.Request) string {
		if u, ok := r.Context().Value(UserContextKey).(*sqlc.User); ok && u != nil && u.ID != "" {
			return "user:" + u.ID
		}
		return "ip:" + ClientIP(r)
	}, scope, limit, window)
}

// rateLimitBy is the shared core of RateLimit and RateLimitUser: keyFn decides the caller's
// identity, the rest (Redis sliding window, RAM fallback, reap) is shared.
func rateLimitBy(keyFn func(*http.Request) string, scope string, limit int, window time.Duration) func(http.Handler) http.Handler {
	l := &keyedLimiter{
		scope:  scope,
		limit:  limit,
		window: window,
		keyFn:  keyFn,
		hits:   make(map[string][]time.Time),
	}
	go l.reap()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(r.Context(), keyFn(r), time.Now()) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"detail":"Too many requests. Please try again later."}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// slidingWindowScript counts and decides in a SINGLE Redis call.
//
// Must be a script rather than a chain of discrete commands: reading the count then writing
// is two steps, and two concurrent requests both read "slot available" before either writes —
// exactly when the limit needs to be precise, it leaks. Redis runs scripts sequentially, so
// there is no such gap.
//
// ZSET with score as the call time: ZREMRANGEBYSCORE trims the portion that has fallen out
// of the window, so this is a true sliding window, not a fixed window.
//
// KEYS[1] key  ARGV[1] cutoff  ARGV[2] current time  ARGV[3] limit  ARGV[4] TTL seconds
// ARGV[5] request-unique string, so two calls at the same microsecond don't overwrite each other
var slidingWindowScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
local used = redis.call('ZCARD', KEYS[1])
if used >= tonumber(ARGV[3]) then
  return 0
end
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[5])
redis.call('EXPIRE', KEYS[1], ARGV[4])
return 1
`)

type keyedLimiter struct {
	scope  string
	limit  int
	window time.Duration
	keyFn  func(*http.Request) string

	mu   sync.Mutex
	hits map[string][]time.Time

	// seq distinguishes two requests arriving at the same nanosecond. Without it, the later
	// ZADD overwrites the previous member instead of adding a new entry, and the limit
	// undercounts.
	seq atomic.Int64
}

// rateLimitKey adds a prefix so keys don't collide with other keys on the same Redis.
func rateLimitKey(scope, key string) string { return "ratelimit:" + scope + ":" + key }

// allow records a call and reports whether it is within the limit.
func (l *keyedLimiter) allow(ctx context.Context, key string, now time.Time) bool {
	if rdb := state.RedisClient; rdb != nil {
		// Do not use the request context: it is cancelled when the client disconnects, and a
		// password guesser who disconnects after each attempt would cause that attempt to
		// not be counted.
		opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheOpTimeout)
		defer cancel()

		res, err := slidingWindowScript.Run(opCtx, rdb,
			[]string{rateLimitKey(l.scope, key)},
			now.Add(-l.window).UnixNano(),
			now.UnixNano(),
			l.limit,
			int(l.window.Seconds())+1,
			strconv.FormatInt(now.UnixNano(), 10)+"-"+strconv.FormatInt(l.seq.Add(1), 10),
		).Int64()

		if err == nil {
			return res == 1
		}
		// Redis error falls through to the in-RAM counter below. Full fail-open would turn a
		// Redis outage into an open door for password guessing; the per-process counter is
		// looser but still blocks, so it is the right choice between the two extremes.
	}

	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Filter in-place to avoid reallocating the slice on every request.
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}

	l.hits[key] = append(kept, now)
	return true
}

// reap cleans up keys that have no remaining traces from the in-RAM counter.
//
// Without it, the map keeps a key for every IP that has ever called and only grows — a cheap
// way to exhaust the RAM of the very server this rate limiter is protecting. The Redis branch
// does not need this: EXPIRE in the script reclaims keys automatically.
func (l *keyedLimiter) reap() {
	for range time.Tick(10 * time.Minute) {
		cutoff := time.Now().Add(-l.window)
		l.mu.Lock()
		for key, ts := range l.hits {
			fresh := false
			for _, t := range ts {
				if t.After(cutoff) {
					fresh = true
					break
				}
			}
			if !fresh {
				delete(l.hits, key)
			}
		}
		l.mu.Unlock()
	}
}

// trustedProxies are the network ranges allowed to set X-Forwarded-For.
//
// Empty means never trust that header. Read-many-write-once: SetTrustedProxies runs exactly
// once at startup, before the router receives its first request.
var trustedProxies []*net.IPNet

// SetTrustedProxies loads the list of trusted proxy ranges from configuration.
//
// An unparseable entry stops the process: a silently-ignored misspelled range means the limit
// keys by the proxy's address — everyone shares one key — or by a forgeable header. Both are
// silent breakage, and a misconfiguration must be visible immediately at startup.
func SetTrustedProxies(cidrs []string) {
	trustedProxies = nil
	for _, c := range cidrs {
		// Bare IPs are also accepted: a single proxy does not need to be written as /32.
		if !strings.Contains(c, "/") {
			if ip := net.ParseIP(c); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				c += "/" + strconv.Itoa(bits)
			}
		}
		_, network, err := net.ParseCIDR(c)
		if err != nil {
			log.Fatalf("TRUSTED_PROXIES: %q is not a valid CIDR: %v", c, err)
		}
		trustedProxies = append(trustedProxies, network)
	}
	if len(trustedProxies) > 0 {
		log.Printf("Trusting X-Forwarded-For from %d proxy range(s)", len(trustedProxies))
	}
}

// ClientIP returns the caller's address, only trusting X-Forwarded-For when the connection
// arrives from a trusted proxy.
//
// X-Forwarded-For can be set by the client. Previously this header was trusted unconditionally,
// so the rate-limit key was literally a value the caller chose: changing the header each time
// made the limit ineffective — measured 200/200 requests passing a 10/min limit, i.e. fully
// bypassed, not "slowed down". That was also the only layer guarding password guessing and
// preventing the server from being forced to hash bcrypt continuously.
//
// Take the RIGHTMOST element that is still outside the trusted ranges, scanning right to left:
// entries to the right are added by our own proxies and can be trusted, while entries to the
// left could have been forged by the client before the request reached the proxy.
func ClientIP(r *http.Request) string {
	remote := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}

	if len(trustedProxies) == 0 || !isTrustedProxy(remote) {
		return remote
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remote
	}

	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		if hop == "" {
			continue
		}
		if net.ParseIP(hop) == nil {
			// A non-IP value means everything to its left is also untrustworthy.
			break
		}
		if !isTrustedProxy(hop) {
			return hop
		}
	}
	return remote
}

// isTrustedProxy reports whether an address falls within the declared proxy ranges.
func isTrustedProxy(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	for _, network := range trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
