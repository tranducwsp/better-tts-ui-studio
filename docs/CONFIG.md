# ⚙️ Environment Variables Configuration Guide

This document explains in detail all Environment Variables used for the Backend Gateway and the **Better TTS UI Studio** system.

---

## 📌 Configuration Management Principle (Single Source of Truth)

All Backend system environment variables are defined centrally in a single location in the source code: the **`backend/config/settings.go`** file.

> [!NOTE]
> The `.env.example` sample file is generated automatically entirely from the declaration table in `settings.go`. As a result, documentation information, default values, and data types are always 100% synchronized with the Go Backend source code.
> 
> If you add or modify a new configuration variable, edit it in `settings.go` and run the following command to update `.env.example`:
> ```bash
> cd backend
> go generate ./config
> ```

---

## 📋 Detailed Environment Variable Reference (ENV Reference)

### 1. Required in Production

| Environment Variable | Type | Default | Description & Instructions |
| :--- | :--- | :--- | :--- |
| `SECRET_KEY` | Secret | (Empty) | Secret key used to sign and verify JWT Session Tokens. **REQUIRED**. Generate key with: `openssl rand -hex 32` |
| `POSTGRES_PASSWORD` | Secret | (Empty) | PostgreSQL database account password. **REQUIRED**. Generate password with: `openssl rand -hex 24` |
| `REDIS_PASSWORD` | Secret | (Empty) | Redis Server access password. Should be set for Production environments. |

---

### 2. Seed Accounts

| Environment Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `DEFAULT_ADMIN_USERNAME` | String | (Empty) | Admin account name that will be automatically created when the application runs for the first time |
| `DEFAULT_ADMIN_PASSWORD` | Secret | (Empty) | Password for the created Admin account |
| `DEFAULT_USER_USERNAME` | String | (Empty) | Sample User account name to create |
| `DEFAULT_USER_PASSWORD` | Secret | (Empty) | Password for the sample User account |

---

### 3. Server & HTTP Gateway Configuration

| Environment Variable | Type | Default | Min/Max Range | Description |
| :--- | :--- | :--- | :--- | :--- |
| `HOST` | String | `0.0.0.0` | - | IP address interface for the HTTP Gateway to listen on |
| `PORT` | String | `8000` | - | Network port of the Backend HTTP Gateway |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | Int | `15` | 1 - 525600 | JWT Access Token expiration time (minutes) |
| `REFRESH_TOKEN_EXPIRE_MINUTES` | Int | `43200` | 60 - 525600 | Refresh Token Cookie expiration time (30 days) |
| `AUTH_USER_CACHE_SECONDS` | Int | `15` | 0 - 3600 | Time to cache User information in RAM to reduce DB query load. Set `0` to disable cache |
| `TASK_MEMORY_RETENTION_SECONDS`| Int | `600` | 0 - 86400 | Time to keep completed Task status in RAM cache. Set `0` to free RAM immediately |
| `ENABLE_REQUEST_LOGGING` | Bool | `true` | true / false | Enable/Disable detailed HTTP request logging to stdout (Disable under high load to optimize RAM) |
| `ENABLE_PPROF` | Bool | `false` | true / false | Enable/Disable Go Profiler endpoint (`/debug/pprof`) for inspecting Heap/Goroutines memory |
| `ENABLE_SWAGGER` | Bool | `true` | true / false | Enable/Disable the OpenAPI / Swagger UI documentation page (`/swagger`) |

---

### 4. PostgreSQL Database Connection

| Environment Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `DATABASE_URL` | String | `postgres://postgres:...@localhost:5432/ai_studio` | PostgreSQL Connection String. When running Docker Compose, this variable is auto-assembled |
| `DB_MAX_CONNS` | Int | `25` | Maximum number of connections in the Connection Pool (`pgxpool`) |
| `DB_MIN_CONNS` | Int | `5` | Minimum number of connections maintained in the Pool (must not exceed `DB_MAX_CONNS`) |
| `DB_MAX_CONN_LIFETIME_MINUTES` | Int | `30` | Maximum lifetime of a DB connection before recreation |
| `DB_MAX_CONN_IDLE_MINUTES` | Int | `15` | Maximum time an idle connection can stay in the pool |
| `DB_CONNECT_MAX_RETRIES` | Int | `10` | Number of DB connection retries when the system starts |
| `DB_CONNECT_RETRY_INTERVAL_SECONDS` | Int | `2` | Wait interval between each DB connection retry |

---

### 5. Queue & Redis Cache Configuration (Redis Optional)

| Environment Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `REDIS_URL` | String | `localhost:6379` | Redis Server connection address. If left empty, the Gateway will run in In-Memory Queue mode |
| `REDIS_PASSWORD` | Secret | (Empty) | Authentication password for Redis Server |

---

### 6. AI Core Engine & Frontend Builder Connection

| Environment Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `CORE_ENGINE_URL` | String | `http://localhost:8001` | HTTP URL of the Python AI Core TTS Engine service (where Manifest & Synthesize are retrieved) |
| `CORE_ENGINE_GRPC_URL` | String | `localhost:50051` | gRPC Server address of the AI Core TTS Engine (if using gRPC) |
| `TTS_CLIENT_TIMEOUT_SECONDS` | Int | `60` | Maximum timeout for a synthesis request call to the AI Core Engine |
| `FE_BUILDER_URL` | String | `http://frontend-builder:3001` | URL of the Node.js Builder service that receives the UI reload webhook when the Manifest changes |
| `VITE_BACKEND_URL` | String | `http://backend:8000` | Backend Gateway address that the Frontend Builder & Dev Server use to read the Manifest |

---

### 7. File Storage & Limits

| Environment Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `STORAGE_BACKEND` | String | `local` | Choose file storage backend: `"local"` (Local disk) or `"s3"` (AWS S3 / MinIO / Cloudflare R2) |
| `STORAGE_DIR` | String | `storage` | Main storage directory when `STORAGE_BACKEND=local` |
| `S3_BUCKET` | String | (Empty) | S3 Bucket name (Required when `STORAGE_BACKEND=s3`) |
| `S3_REGION` | String | `us-east-1` | Region of the S3 Bucket |
| `S3_ENDPOINT` | String | (Empty) | Custom S3 endpoint (e.g., `https://s3.example.com` for MinIO/R2) |
| `S3_ACCESS_KEY_ID` | Secret | (Empty) | Access Key ID for S3 |
| `S3_SECRET_ACCESS_KEY` | Secret | (Empty) | Secret Access Key for S3 |
| `S3_FORCE_PATH_STYLE` | Int | `0` | Set `1` when using MinIO (path-style access `endpoint/bucket/key`) |
| `S3_PREFIX` | String | (Empty) | Directory prefix on S3 to distinguish environments (`prod`, `staging`) |
| `MAX_UPLOAD_SIZE_MB` | Int | `50` | Hard cap on maximum file upload size (MB) for the Gateway infrastructure |
| `TEMP_AUDIO_RETENTION_HOURS` | Int | `24` | Number of hours to retain temporary audio files in `storage/temp/` before the Sweeper deletes them |
| `PRESERVE_FILES` | Int | `0` | Set `1` to keep sample audio files on disk when deleting a Clone voice in the DB |

---

### 8. Network Security & Advanced Optimization

| Environment Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `COOKIE_SECURE` | Int | `1` | Set `1` to enable the `Secure` flag for Cookies (only sent over HTTPS). Set `0` when developing on `http://localhost` |
| `CORS_ALLOWED_ORIGINS` | List | `http://localhost:5173,...` | List of domains allowed to access the API (comma-separated) |
| `TRUSTED_PROXIES` | List | (Empty) | Trusted Proxy CIDR ranges (`10.0.0.0/8`) allowed to set the `X-Forwarded-For` header |
| `WORKER_MAX_IN_FLIGHT` | Int | `2` | Maximum number of concurrent synthesis jobs in each Worker process |
| `TRANSCODE_MAX_CONCURRENCY` | Int | `2` | Maximum number of concurrent `ffmpeg` audio transcoding processes |
| `STALE_CHUNK_AFTER_MINUTES` | Int | `30` | Maximum minutes for a Task Chunk to be stuck before the Sweeper marks it as `failed` |
| `AUTH_RATE_LIMIT_REQUESTS` | Int | `10` | Maximum number of allowed Login/Register API calls in a window |
| `AUTH_RATE_LIMIT_WINDOW_SECONDS` | Int | `60` | Length of the Rate Limit window for Login/Register (seconds) |