# AGENT.md — Coding Agent Guide

## Scope

This file applies to the entire repository. If a directory has its own guide file in the future, the guide closest to the file being modified takes precedence.

## Product Goals

Better TTS UI Studio is a **Web UI and schema-driven control plane** for AI Engineers to deploy a TTS engine as a product. This repository owns the interface, gateway, auth, queue, storage, and operations; it does not own the model/inference part.

- Keep the platform independent of specific models and engines.
- UI and gateway must derive model, capability, constraints, and dynamic components from the manifest `GET /info`.
- Do not hard-code model names, voices, modes, or engine-specific parameters into shared flows.
- The product competes with Gradio at the UI/control plane layer for model deployment, not at the training or inference layer.

## Mandatory Constraints

### Do Not Modify Core TTS

`core-tts-example/` is only a stub/reference for the integration contract. The real Core TTS resides in a repository managed by the AI Engineer.

- **Do not modify any file in `core-tts-example/`.**
- If a requirement needs changes on the engine side, describe the contract/behavior the engine needs to provide rather than implementing it here.
- Protocol or manifest changes must maintain compatibility between the platform and the engine outside this repository; do not break the existing contract arbitrarily.

### Deploy Using Docker Compose

The standard deployment workflow for this repository is `docker compose`; do not build or run binaries/images manually to replace this process.

```bash
cp .env.example .env   # only on first setup; fill in real secrets

docker compose config
docker compose up -d --build
docker compose ps
docker compose logs -f backend worker frontend-builder frontend
```

Use `docker compose down` when the stack needs to be stopped. Do not use `down -v` unless the user confirms they want to delete PostgreSQL data.

### Secrets and Configuration

- Do not commit `.env`, secrets, tokens, passwords, S3 credentials, or user data.
- `backend/config/settings.go` is the **single source of truth** for backend configuration.
- `.env.example` is an auto-generated file. When changing a setting, modify `backend/config/settings.go`, update `LoadConfig`, then run:

  ```bash
  cd backend
  go generate ./config
  ```

- Local HTTP typically requires `COOKIE_SECURE=0`; production behind HTTPS must keep `COOKIE_SECURE=1`.
- Do not expose Postgres, Redis, engine, or frontend-builder to the host without a clear architectural requirement. They are internal services within Compose.

## Repository Structure

- `backend/`: Go control plane, API, auth, queue, worker, cron, storage, and database.
- `frontend/`: Svelte 5 + TypeScript + Vite SPA/prerendered UI; not SvelteKit.
- `core-tts-example/`: read-only stub contract, do not modify.
- `k8s/`: Helm/Kubernetes deployment manifests.
- `tests/`: Test suites, fixtures, k6 smoke/load/soak tests, monitoring, and reports.
      - `tests/load/`: k6 load test scripts.
      - `tests/monitoring/`: Python monitoring scripts.
      - `tests/reports/`: HTML/JSON metrics reports.
      - `tests/fixtures/`: Shared test fixtures (shared across Go + frontend).
- `docs/`: architecture, backend, frontend, gateway, and configuration documentation.
- `docker-compose.yml`: standard integrated run/deploy method.

Read `README.md` and related documentation in `docs/` before changing flows that span multiple services. Check the current code if documentation and implementation contradict; update outdated documentation in the same change when appropriate.

## Backend Conventions

- Go modules reside in `backend/`; the Go version in `backend/go.mod` is the authoritative source.
- Processes have distinct responsibilities:
  - `cmd/web`: HTTP API/control plane.
  - `cmd/worker`: consume Redis stream and run synthesis pipeline.
  - `cmd/cron`: clean up temp files and stale chunks; run only one replica.
- Do not put long-running inference in request handlers. Web enqueues work, the worker processes it, and the client tracks progress via SSE.
- Maintain authorization/ownership checks for every user resource: job, chunk, history, voice, and audio.
- Maintain the `storage.Store` abstraction; business code must not depend directly on the local filesystem when S3 also needs to work.
- Redis has an in-memory fallback for single-replica deployments. Do not assume this fallback supports multi-replica semantics.
- Use existing error/JSON response helpers and keep the comment style, naming, and package layout of surrounding code.
- Run `gofmt` on every modified Go file.

### Database and Auto-Generated Code

- Every runtime schema change requires new `up` and `down` migrations in `backend/db/migrations/`; do not modify already-released migrations to alter history.
- Keep `backend/db/schema.sql` in sync with the final schema that queries/codegen need to see.
- Source SQL queries reside in `backend/db/query/`.
- `backend/db/sqlc/` is auto-generated code; do not modify manually. After changing schema or queries, run from `backend/`:

  ```bash
  sqlc generate
  ```

- `backend/proto/tts.pb.go` and `backend/proto/tts_grpc.pb.go` are generated files; do not modify manually. Only change the protobuf contract when the requirement has been coordinated with the engine outside the repository, then regenerate using the appropriate protobuf toolchain.

## Frontend Conventions

- Use Svelte 5 runes and TypeScript following existing patterns; do not introduce SvelteKit/router/framework new state without an approved reason.
- Keep the UI schema-driven. Dynamic components, ranges, text limits, audio specs, and capabilities must derive from the manifest or shared helpers.
- Authenticated API calls must use `authFetch` in `frontend/src/lib/api.ts` to preserve cookies and refresh-token retry. Do not use bare `fetch` for protected endpoints.
- Keep requests relative (`/api/...`, `/storage/...`) so the Vite/Nginx proxy decides the backend target.
- When adding purely logic behavior, prefer extracting a TypeScript helper and adding a `*.test.ts` test rather than embedding all logic in a `.svelte` component.
- Do not modify or commit runtime/build output such as `frontend/dist/`, `frontend/node_modules/`, or data in `backend/storage/`.

## Test Commands

Run the minimal relevant checks first, then the full suite for the modified area.

### Backend

```bash
cd backend
gofmt -w <modified-go-files>
go test ./...
go vet ./...
go build ./cmd/...
```

`go test ./...` is the minimum check for backend changes. If config is changed, tests must also confirm `.env.example` is still in sync. If queries/schema are changed, run `sqlc generate` before tests and check the generated diff.

### Frontend

```bash
cd frontend
npm ci
npm run check
npm test
npm run build
```

Use `npm ci` when dependencies need to be installed from the lockfile; do not change `package-lock.json` if dependencies are unchanged.

### Integration

After changes touching the contract between frontend/backend, queue, storage, or deployment:

```bash
docker compose config
docker compose up -d --build
docker compose ps
curl -f http://localhost:8000/health
```

### Playwright (UI Integration)

Run Playwright tests against the running Docker stack to verify manifest-to-UI consistency:

```bash
# Install Playwright browsers (first time only)
npx playwright install chromium

# Run all integration tests
npx playwright test

# Run a specific test file
npx playwright test tests/integration/manifest_ui.spec.ts

# Run tests in headed mode (watch the browser)
npx playwright test --headed

# Run tests with UI mode (interactive dashboard)
npx playwright test --ui
```

Smoke test with k6 when the stack is ready:

```bash
docker run --rm -i --network=host \
  -v "$(pwd)/tests/load:/tests/load" grafana/k6 run /tests/load/smoke.js
```

Do not run load/stress/soak tests unless the user requests them: they consume resources and may generate a lot of data.

## Test Layout

Every test file must reside in the corresponding `tests/` directory — do not place test files next to source code:

- `backend/tests/{unit,integration,contract}/` for Go tests
- `frontend/tests/unit/` for TypeScript/vitest tests
- `tests/{load,monitoring,reports,fixtures}/` for k6, Python, reports, fixtures

Check layout:

```bash
scripts/check-test-layout.sh
```

## Cleanup After Testing

Testing generates data that accumulates quickly and can fill disk space. Run cleanup after every test session, especially before switching branches or leaving the project.

### ⚠️ Known Issues

- **`backend/storage/voice/` may not exist** — always use `rm -rf backend/storage/voice` (no trailing glob) to handle missing dir gracefully.
- **Root-owned files** — Docker containers create files as root in `storage/` subdirectories. Use `docker compose run --user root` to clean them (passwordless sudo is not available).

### Storage Cleanup

```bash
# Clean generated audio files (respects .gitignore patterns)
# Use NULL_GLOB to avoid zsh "no matches found" when dir is empty
setopt NULL_GLOB 2>/dev/null; rm -rf backend/storage/audio/* backend/storage/temp/*; setopt NO_NULL_GLOB 2>/dev/null

# Clean voice/ (may not exist; use bare path, no wildcard)
rm -rf backend/storage/voice

# Clean root-owned files (created by Docker containers)
docker compose run --user root --entrypoint sh backend \
  -c "rm -rf /app/storage/019f90a6-d5c8-7795-bae5-de6ae408d880 /app/storage/5fa2daa6-eb1d-4262-b185-2937b0a3cba5 /app/storage/clone/*"
```

### Docker Cleanup

The build cache is the biggest space hog. After several `docker compose up --build` iterations, it can grow to **40+ GB**.

```bash
# Stop the stack (use -v ONLY if you want to delete PG data too)
docker compose down

# Remove dangling images
docker image prune -f

# ⚠️ CRITICAL: Prune build cache — typically reclaims 40+ GB
docker builder prune -f

# Remove orphan containers (left over from docker compose run)
docker compose down --remove-orphans

# Remove unused volumes (reclaim 100-500 MB)
docker volume prune -f

# Full cleanup (when disk is critically low)
docker system prune -af --volumes   # CAUTION: removes everything
```

### Disk Usage Check

```bash
# Check how much space storage is using
du -sh backend/storage/*/

# Check Docker disk usage (images, containers, build cache, volumes)
docker system df

# Check overall disk
df -h /
```

### Redis Cleanup (test keys)

```bash
# Flush test data (Redis requires password from .env)
docker compose exec redis redis-cli -a "${REDIS_PASSWORD:-devredis}" FLUSHDB
```

### PostgreSQL Cleanup (test data)

```bash
# Remove test users and history (adjust for your test patterns)
docker compose exec postgres psql -U user -d aitts -c "DELETE FROM users WHERE username LIKE 'test%';"
docker compose exec postgres psql -U user -d aitts -c "DELETE FROM history WHERE 1=1;"  # CAUTION
```

### k6 / Load Test Cleanup

```bash
# Remove reports and metrics
rm -f tests/reports/*.json tests/reports/*.html tests/monitoring/*.json
```

### Log Cleanup

```bash
# Truncate container logs (prevents Docker log files from filling disk)
docker compose logs -f > /dev/null 2>&1  # or:
truncate -s 0 $(docker inspect --format='{{.LogPath}}' $(docker compose ps -q) 2>/dev/null) 2>/dev/null
```

### Quick One-Liner (full cleanup after stopping the stack)

```bash
# Stop stack, clean storage, prune images + build cache + volumes + orphans
docker compose down && \
  find backend/storage/audio/ backend/storage/temp/ -type f -delete 2>/dev/null; \
  rm -rf backend/storage/voice 2>/dev/null; \
  docker compose run --user root --entrypoint sh backend \
    -c "rm -rf /app/storage/clone/*" 2>/dev/null; \
  docker image prune -f && \
  docker builder prune -f && \
  docker volume prune -f && \
  docker compose down --remove-orphans
```

## Pre-Completion Checklist

1. Changes are within the scope of the requirement; no unrelated refactoring.
2. No file in `core-tts-example/` has been modified.
3. No secrets, `.env`, runtime data, or generated build output have been added to Git.
4. New behavior has tests or a clear explanation of why automated testing is not yet possible.
5. Appropriate test commands have been run and accurately reported as pass/fail/skipped.
6. Schema changes include migrations; query/config changes have correctly regenerated artifacts from the source.
7. API/manifest/deployment changes include updates to related documentation when the contract has changed.
8. Schema-driven flow, auth, ownership, storage abstraction, and service boundaries are preserved.