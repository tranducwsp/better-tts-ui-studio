# 🔧 Backend Technical Specification

This document specifies the detailed technical architecture of the **Control Plane Gateway** (written in **Go - Golang 1.22**), including architectural layers, Concurrency processing mechanisms, JWT security, Database connections, Task Queue, and storage integration.

---

## 🛠️ 1. Technology & Libraries Used (Tech Stack)

| Component | Technology / Library | Description & Optimization |
| :--- | :--- | :--- |
| **Language** | **Go 1.22** | High-performance compiled language, optimal RAM memory management and superior Goroutines concurrency |
| **HTTP Router** | **Chi Router (`go-chi/chi/v5`)** | Lightweight, idiomatic Go HTTP router, fast routing speed, flexible Middleware chaining support |
| **JSON Parser** | **Sonic (`bytedance/sonic`)** | Fastest JIT-compiled JSON serializer/deserializer library in the Go ecosystem |
| **Database Pool** | **`pgx/v5/pgxpool`** | PostgreSQL driver communicating directly via binary protocol, high-concurrency connection pool management |
| **Task Queue** | **Redis Streams (`go-redis/v9`)** | Reliable job queue (At-least-once processing, Consumer Groups), automatic fallback to Go Channel RAM if Redis is unavailable |
| **Storage SDK** | **`aws-sdk-go-v2`** | Official SDK for communicating with AWS S3, MinIO, Cloudflare R2 |
| **Auth Security** | **JWT (`golang-jwt/jwt/v5`) + Bcrypt** | Secure password hashing and Access/Refresh Token signing stored in HttpOnly Cookies |
| **Documentation** | **HttpSwagger (`swaggo/http-swagger`)** | Serving dynamic Swagger API documentation |

---

## 🏗️ 2. Source Code Layers (Layered Architecture)

The Backend source code is divided into independent module layers, applying **Clean Architecture** principles:

```
        ┌─────────────────────────────────────────┐
        │            HTTP Router & Config         │
        └────────────────────┬────────────────────┘
                             │
        ┌────────────────────▼────────────────────┐
        │         Middlewares (Auth, Limit)       │
        └────────────────────┬────────────────────┘
                             │
        ┌────────────────────▼────────────────────┐
        │              API Handlers               │
        └──────────┬───────────────────┬──────────┘
                   │                   │
  ┌────────────────▼──────────┐ ┌──────▼───────────────────┐
  │ Synth Task Queue & Worker │ │  Storage Engine (S3/Disk) │
  └────────────────┬──────────┘ └──────────────────────────┘
                   │
  ┌────────────────▼──────────┐ ┌───────────────────────────┐
  │ Core TTS Client (HTTP/gRPC)│ │  PostgreSQL Database      │
  └───────────────────────────┘ └───────────────────────────┘
```

---

## 🔒 3. Authentication & Security

### 3.1. JWT Session Mechanism with HttpOnly Cookies

The system implements a dual secure authentication mechanism:

- **Access Token**: Short lifespan (Default `15 minutes`), stored in an HttpOnly Cookie named `access_token`. Used to authenticate all API requests.
- **Refresh Token**: Long lifespan (Default `30 days`), stored in an HttpOnly Cookie named `refresh_token` with a dedicated path `/api/auth/refresh`.
- **HttpOnly & Secure Flags**: Cookies are set with the `HttpOnly` flag (prevents theft via XSS attacks) and `SameSite=Lax`. The `Secure` flag is automatically enabled on HTTPS protocol (`COOKIE_SECURE=1`).

### 3.2. Password Hashing & Account Protection

- User passwords are hashed using the **Bcrypt** algorithm before being stored in PostgreSQL.
- **Rate Limiting**: Sensitive endpoints like `/api/login` and `/api/register` are protected by a Rate Limit Middleware (`AUTH_RATE_LIMIT_REQUESTS`, default 10 requests/minute) to prevent brute-force password attacks.
- **Role-Based Access Control (RBAC)**: The system is divided into 2 main roles: `user` and `admin`. Newly registered accounts must go through `pending` status and be approved by an Admin via `/api/admin/users/{user_id}/approve` before they can use synthesis features.

---

## ⚡ 4. Queue Coordination & Concurrency Management (Task Pipeline)

### 4.1. Job Initialization & Chunking

When a client sends a synthesis request:
1. `handlers/history.go`: Receives the `POST /api/jobs/init` request, records a new Job record in the `jobs` table in PostgreSQL.
2. `synth/pipeline.go`: Splits long text into small Chunks based on the Manifest's `max_chars` limit.
3. Creates corresponding Task Chunk records in the `tasks` table with `pending` status.

### 4.2. Queue Push & Concurrent Processing

1. `queue/redis_stream.go`: Pushes Task IDs into the Redis Stream `tts:tasks`.
2. Background Worker Goroutines running in `synth/pipeline.go` listen to the Stream via Redis Consumer Group:
   - Limit concurrent Task processing by `WORKER_MAX_IN_FLIGHT`.
   - Fetch Task -> Transition status to `processing` -> Call AI Core TTS Engine.
   - Receive output audio stream -> Pipe through `ffmpeg` for transcoding (if needed) -> Save file to Storage -> Mark status as `completed`.

---

## 💾 5. Storage Layer Abstraction

The source code defines a single Interface in `storage/store.go`:

```go
type Store interface {
    Save(ctx context.Context, key string, r io.Reader) (string, error)
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
}
```

The system provides 2 implementations:
1. **`LocalStorage` (`storage/local.go`)**: Read/Write files directly on the server's local disk (`STORAGE_DIR`). Suitable for testing or single-node deployments.
2. **`S3Storage` (`storage/s3.go`)**: Read/Write files on Cloud Object Storage systems (AWS S3, MinIO, Cloudflare R2). Suitable for multi-Pod/Replica scaling on Kubernetes.

---

## 📊 6. RAM Memory Management & Performance Tuning

To maintain stability in Production environments with thousands of concurrent requests:

1. **User Cache In-Memory**: Cache User records in temporary memory (`AUTH_USER_CACHE_SECONDS`) to reduce 90% of repetitive `SELECT` queries to PostgreSQL.
2. **Body Limit Middleware (`middleware/body_limit.go`)**: Cap the maximum HTTP request body size at the Router layer, rejecting requests with excessively large `Content-Length` early (prevents RAM exhaustion).
3. **Concurrency Limiter (`middleware/concurrency.go`)**: Set a ceiling on the number of concurrent heavy connections (e.g., `/api/extract-text` limits to a maximum of `8` concurrent requests because each PDF/DOCX file extraction request consumes ~32MB RAM).
4. **Stale Task Sweeper (`database/sweeper.go`)**: Runs in the background periodically to reclaim and report errors for Tasks stuck for too long due to sudden Worker death or network loss.