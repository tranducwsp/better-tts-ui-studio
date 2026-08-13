# CACHE Architecture

This document describes what is cached, where (Redis vs in-memory), and why.

## Redis

Redis is the authoritative sidecache for cross-process state. Every field below is a Redis key with a bounded TTL.

| What | Key Pattern | TTL | Why Redis, not RAM |
| :--- | :--- | :--- | :--- |
| **Task state** | `task:{task_id}` | 5 min after done | Worker writes progress, API server reads it — two processes. |
| **Audio output** | `audio:{task_id}` | 5 min after done | Same reason: worker writes the blob, the API server streams it to the client. |
| **Pub/Sub (task events)** | N/A (channels) | — | Workers notify the API server that a task moved to done/failed without polling. |
| **Queue (per-engine)** | `queue:{engine}` | — | List of pending task IDs. Survives a restart of either process. |
| **Rate-limit counters** | `ratelimit:{user_id}:{endpoint}` | window | Sliding-window state must be shared across all replicas; in-process counters would reset per-pod. |
| **User cache** | `auth:user:{username}` | 5 s | Very short TTL: just enough to absorb a burst of requests without hitting PostgreSQL on every one. See [user_cache.go](backend/middleware/user_cache.go). |
| **Online status** | `online:{user_id}` | 60 s | Touched on every authenticated request. Used only for the admin user list. |

### Why not in-memory for these?

- **Task state & audio**: The worker pod and the API pod are different processes (or different hosts). In-memory would mean the API can never see what the worker produced.
- **Queue & rate-limit**: Must survive pod restarts and be visible to all replicas.
- **User cache**: Needs to be invalidated globally when an admin approves an account. An in-memory-only cache on replica A would still serve the stale `is_approved=false` after replica B processed the approval. The TTL is short enough that a Redis outage just means a few extra SELECTs.

## In-Memory (Go `sync.Map` / `map` \+ `sync.RWMutex`)

These are hot singletons where cross-process sharing provides no benefit, and the extra latency of Redis would be pure overhead.

| What | Type | Why not Redis |
| :--- | :--- | :--- |
| **Manifest** (engine info) | `backend/state.ManifestCache` | Written once at startup, read on every page load. Redis round-trip would add ~1 ms per request for data that never changes until the next deploy. |
| **TaskManager** (in-memory job registry) | `backend/state.TaskManager` | Tracks running tasks by their `*Task` handle. Only the process that owns the worker pool needs these pointers. |
| **PresetVoiceCache** | `backend/state.PresetVoiceCache` | Pre-computed voice list from the manifest, built once and served for the lifetime of the process. |
| **Concurrency limiter** | `backend/middleware/ratelimit.go` | Per-process semaphore that bounds how many concurrent requests hit the engine. Cross-process coordination would defeat the purpose of a per-pod limit. |

## Data Flow

```
Browser ──HTTP──▶ Gateway (API) ──gRPC──▶ Engine Worker
                    │   ▲
                    │   │
               Redis ◀──┘  (task state, audio, rate limit…)
                    │
                    ▼
              PostgreSQL  (users, history, voice metadata)
                    ▲
                    │
          User Cache (Redis, 5 s TTL)
```

1. The browser sends a synthesis request to the Gateway API.
2. The API validates the user (user cache → Redis, fallback → PostgreSQL).
3. The API enqueues the task in Redis (`queue:{engine}`) and returns a task ID.
4. The engine worker pops from the queue, processes, and writes progress to Redis (`task:{task_id}`).
5. The browser polls the task endpoint (or subscribes via SSE), which reads from Redis.
6. When done, the browser downloads the audio from Redis (`audio:{task_id}`).

## Config Reference

| Env Var | Default | Used For |
| :--- | :--- | :--- |
| `REDIS_ADDR` | `localhost:6379` | Address of the Redis server. |
| `REDIS_PASSWORD` | `""` | Optional Redis ACL password. |
| `REDIS_DB` | `0` | Redis database number. |
| `CACHE_TTL_USER` | `5s` | TTL for the user cache entries. |
| `CACHE_TTL_TASK` | `5m` | TTL for completed task state and audio. |
| `RATE_LIMIT_WINDOW` | `1m` | Rate-limit sliding window duration. |
| `RATE_LIMIT_MAX` | `30` | Max requests per window per user. |

## Key Design Principles

1. **Redis is the source of truth for ephemeral cross-process state.** PostgreSQL is the source of truth for durable data.
2. **In-memory is for hot singletons.** If the data is per-process, never changes, or is a pointer to a running goroutine, it stays in RAM.
3. **No dual-write to both Redis and RAM for the same data.** This was a source of subtle bugs in earlier versions of the user cache, where a stale RAM copy survived Redis invalidation.
4. **Short TTLs.** User cache entries live for 5 seconds — long enough to absorb a burst, short enough that a stale entry is harmless.
5. **Redis unavailability degrades to PostgreSQL, not to errors.** Every cache miss path falls through to the database. Timeouts are treated as cache misses.