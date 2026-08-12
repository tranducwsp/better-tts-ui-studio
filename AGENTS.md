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

## Pre-Completion Checklist

1. Changes are within the scope of the requirement; no unrelated refactoring.
2. No file in `core-tts-example/` has been modified.
3. No secrets, `.env`, runtime data, or generated build output have been added to Git.
4. New behavior has tests or a clear explanation of why automated testing is not yet possible.
5. Appropriate test commands have been run and accurately reported as pass/fail/skipped.
6. Schema changes include migrations; query/config changes have correctly regenerated artifacts from the source.
7. API/manifest/deployment changes include updates to related documentation when the contract has changed.
8. Schema-driven flow, auth, ownership, storage abstraction, and service boundaries are preserved.