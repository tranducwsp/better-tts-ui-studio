# Commands Reference

This document catalogs every command used across the codebase — shell scripts, Dockerfiles, build tools, Go binaries, npm scripts, Kubernetes templates, and testing commands. Use it as a quick reference for development, debugging, and CI setup.

---

## 1. Go Binaries

Four binaries are built from `backend/cmd/`:

| Binary | Source | Purpose |
|--------|--------|---------|
| `web` | `backend/cmd/web/main.go` | HTTP server — Chi router, serves API on port 8000 |
| `worker` | `backend/cmd/worker/main.go` | Synthesis worker — consumes tasks from Redis Stream |
| `cron` | `backend/cmd/cron/main.go` | Cron job — sweeps stale chunks, cleans expired temp audio |
| `gen-env` | `backend/cmd/gen-env/main.go` | Developer tool — generates `.env.example` from `config/settings.go` |

**Build:**
```bash
go build -o web    ./cmd/web
go build -o worker ./cmd/worker
go build -o cron   ./cmd/cron
```

> `gen-env` is a developer-only tool, not a runtime binary. It is invoked via `go run` (see §5).

---

## 2. Dockerfiles

### 2.1. `backend/Dockerfile`

Multi-stage build for the Go backend.

```dockerfile
# Build stage
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o web    ./cmd/web
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o cron   ./cmd/cron

# Runtime stage
RUN apk add --no-cache ca-certificates ffmpeg
RUN adduser -D -u 1000 appuser
```

**Runtime dependencies:** `ffmpeg` (audio transcoding), `ca-certificates` (S3/HTTPS calls).

### 2.2. `frontend/Dockerfile`

Serves the compiled Svelte app via nginx.

```dockerfile
RUN npm install
RUN npm run build
CMD ["nginx", "-g", "daemon off;"]
```

### 2.3. `frontend/Dockerfile.builder`

Listens for webhook calls and rebuilds the static bundle.

```dockerfile
RUN npm install
CMD ["npm", "run", "builder"]
```

### 2.4. `core-tts-example/Dockerfile`

Example Python TTS engine.

```dockerfile
RUN pip install --no-cache-dir -r requirements.txt
CMD ["python", "main.py"]
```

---

## 3. Docker Compose

Defined in `docker-compose.yml` — 6 services:

| Service | Image | Command | Healthcheck |
|---------|-------|---------|-------------|
| `postgres` | `postgres:16-alpine` | (default) | `pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}` |
| `redis` | `redis:7-alpine` | `redis-server --requirepass ${REDIS_PASSWORD}` | `redis-cli -a ${REDIS_PASSWORD} ping` |
| `core-engine` | `core-tts-example` | `python main.py` | — |
| `backend` | `studio-backend` | `./web` | — |
| `worker` | `studio-backend` | `./worker` | — |
| `cron` | `studio-backend` | `./cron` | — |

**Deploy:**
```bash
cp .env.example .env
docker compose up -d --build
```

---

## 4. Go `exec.Command` Calls

### 4.1. ffmpeg transcoding — `backend/audio/transcode.go`

```go
exec.CommandContext(ctx, "ffmpeg", full...)
```

Supported output formats and their ffmpeg arguments:

| Format | Arguments |
|--------|-----------|
| WAV | `-f wav -ac 1 -ar 24000 pipe:1` |
| MP3 | `-f mp3 -b:a 128k pipe:1` |
| OGG | `-f ogg -b:a 64k pipe:1` |

### 4.2. gen-env test — `backend/tests/contract/config/settings_test.go`

```go
exec.Command("go", "run", "./cmd/gen-env")
```

Verifies that the committed `.env.example` matches the current config spec table.

---

## 5. `.env.example` Generation

The `.env.example` file is auto-generated from the configuration spec table in `backend/config/settings.go`. It is **not** hand-maintained.

### 5.1. Generate

Run from the repository root:

```bash
cd backend && go generate ./config
```

Or equivalently, without `go:generate`:

```bash
cd backend && go run ./cmd/gen-env
```

The output is written to `.env.example` at the repository root. The generated file is committed so operators can read it without Go tooling.

### 5.2. Verify

The test `backend/tests/contract/config/settings_test.go` verifies that the committed `.env.example` matches the current spec table:

```go
// Under the hood it runs:
cmd := exec.Command("go", "run", "./cmd/gen-env")
```

If the test fails, re-run `go generate ./config` and commit the updated file.

### 5.3. Why a dedicated binary?

The `gen-env` tool is pure Go stdlib with no external dependencies. It reads the `config.Settings` slice and prints each variable with its doc comment, type, default, and group. A separate binary (rather than a template engine or shell script) ensures:
- The example file is always in sync with the actual defaults in `config/settings.go`
- Variable metadata (required, secret, read-by) is rendered correctly
- Secret defaults are never written into the example file

---

## 6. Shell Scripts

### 6.1. `scripts/check-test-layout.sh`

Validates that test files are not placed alongside source code.

```bash
# Backend: ensure no *_test.go outside backend/tests/
find "$ROOT/backend" -name '*_test.go' -not -path '*/tests/*'

# Frontend: ensure no *.test.ts outside frontend/tests/
find "$ROOT/frontend" -name '*.test.ts' -not -path '*/tests/*' -not -path '*/node_modules/*'

# Root: ensure no *.js or *.py test scripts at the repo root
find "$ROOT" -maxdepth 1 \( -name '*.js' -o -name '*.py' \)
```

### 6.2. `core-tts-example/scripts/build_proto.sh`

Compiles the protobuf definition into Python stubs.

```bash
python3 -m venv venv
source venv/bin/activate
pip install -q grpcio-tools
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. proto/tts.proto
```

### 6.3. `core-tts-example/scripts/run_example.sh`

Launches the example TTS engine locally.

```bash
python3 -m venv venv
source venv/bin/activate
pip install -q -r requirements.txt
python main.py
```

---

## 7. npm Scripts

Defined in `frontend/package.json`:

| Script | Command |
|--------|---------|
| `prebuild` | `mkdir -p public/fontawesome public/fonts && cp -r node_modules/@fortawesome/fontawesome-free/css node_modules/@fortawesome/fontawesome-free/webfonts public/fontawesome/ && cp -r node_modules/@fontsource/inter/files public/fonts/` |
| `dev` | `vite` |
| `build` | `npm run prebuild && vite build && node scripts/prerender.js` |
| `builder` | `node scripts/builder_server.js` |
| `preview` | `vite preview` |
| `check` | `svelte-check --tsconfig ./tsconfig.app.json && tsc -p tsconfig.node.json` |
| `test` | `vitest run` |
| `test:watch` | `vitest` |

---

## 8. Node.js Build Scripts

### 8.1. `frontend/scripts/builder_server.js`

HTTP server (port 3001) that listens for webhook `POST /rebuild` and triggers a full rebuild:

```js
exec('npm run build', { cwd: path.resolve('.') })
```

### 8.2. `frontend/scripts/prerender.js`

SSG prerender script that:
1. Fetches the manifest from the backend (`GET /api/info`)
2. Renders the Svelte app to static HTML
3. Runs PurgeCSS on FontAwesome CSS
4. Inlines all CSS into `<head>`
5. Moves JS module to the bottom of `<body>`

---

## 9. Kubernetes Templates

Located in `k8s/templates/` — Helm charts.

| Template | Command | Notes |
|----------|---------|-------|
| `deployment-backend.yaml` | (default: `web`) | Image: `studio-backend` |
| `deployment-worker.yaml` | `command: ["./worker"]` | Image: `studio-backend` |
| `cronjob.yaml` | `command: ["./cron"]` | Schedule from `values.yaml` |
| `deployment-engine.yaml` | (default: `python main.py`) | Image: `studio-engine` |
| `deployment-frontend.yaml` | (default: nginx) | Image: `studio-frontend` |
| `deployment-frontend-builder.yaml` | (default: `npm run builder`) | Image: `studio-frontend-builder` |

**Image registry:** `your-registry.example.com/`

---

## 10. Python Monitoring Scripts

Located in `tests/monitoring/` — collect Docker stats for performance analysis.

```python
subprocess.run(["docker", "stats", "--no-stream", "--format",
                "{{.Name}}|{{.MemUsage}}|{{.CPUPerc}}",
                "ai-backend", "ai-worker", "ai-engine"])
```

| Script | Duration |
|--------|----------|
| `monitor_quick.py` | 30 seconds |
| `monitor_1h.py` | 1 hour |
| `monitor_2h.py` | 2 hours |
| `monitor_5h.py` | 5 hours |

---

## 11. Load Testing (k6)

Located in `tests/load/`. Run with k6 or Docker:

```bash
# Smoke test (1 VU)
docker run --rm -i --network=host -v $(pwd)/tests/load:/tests/load \
  grafana/k6 run /tests/load/smoke.js

# API test
docker run --rm -i --network=host -v $(pwd)/tests/load:/tests/load \
  grafana/k6 run /tests/load/api.js

# Load test (10k VUs)
k6 run tests/load/load.js

# Stress test
k6 run -e STAGE=stress tests/load/load.js

# Soak test (3k VUs, 2 hours)
k6 run -e STAGE=soak tests/load/load.js

# Custom backend URL
k6 run -e BASE_URL=http://localhost:8000 tests/load/smoke.js

# Output to JSON
k6 run --out json=results.json tests/load/load.js
```

---

## 12. System Dependencies

These tools must be available at runtime or build time:

| Tool | Used By | Stage |
|------|---------|-------|
| `go` 1.22+ | Backend build, `go generate`, tests | Build |
| `node` 22+ | Frontend build, dev server, tests | Build |
| `npm` 11+ | Frontend dependency management | Build |
| `sqlc` | Regenerating `backend/db/sqlc/` from `backend/db/query/*.sql` | Build |
| `docker` | Deploy, monitoring scripts | Deploy / Runtime |
| `docker compose` | Local deploy | Deploy |
| `ffmpeg` | Audio transcoding in backend | Runtime |
| `nginx` | Frontend static file serving | Runtime |
| `redis-cli` | Healthcheck in docker-compose | Runtime |
| `pg_isready` | Healthcheck in docker-compose | Runtime |
| `python3` | TTS engine, monitoring scripts, protobuf compile | Build / Runtime |
| `k6` | Load testing | Test |
| `helm` / `kubectl` | Kubernetes deployment | Deploy |