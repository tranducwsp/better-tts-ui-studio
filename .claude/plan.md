# Plan: Redo gRPC Implementation (Complete Rewrite)

## Overview
Rewrite the stale gRPC layer to match the current contract. The old proto has wrong fields (`voice` → `voice_id`), missing fields (`engine`, `pitch`, `emotion`), uses server-streaming (engine returns all audio at once), and is never wired into the running application.

## Key Design Decisions
- **Single `Synthesize` RPC** — the `engine` field already selects mode (standard/fast/clone). Three separate RPCs = three near-identical methods on both sides.
- **Unary, not streaming** — both `synthesize_standard_sync` and `synthesize_fast_async` return complete audio in one shot.
- **gRPC for Synthesize only** — Manifest/Voices/Clone stay HTTP (lightweight, infrequent, no benefit from gRPC).
- **Fallback at startup** — if gRPC connection fails, log warning and continue HTTP-only. No per-request fallback.
- **50MB message limit** — gRPC default is 4MB; audio can exceed this.

## Steps

### 1. Update proto file (both copies)
- `core-backend/proto/tts.proto` and `core-tts/proto/tts.proto`
- Single `Synthesize` RPC, `SynthesizeRequest` with `text`, `voice_id`, `speed`, `engine`, `optional pitch`, `optional emotion`
- `SynthesizeResponse` with `audio_bytes`, `error_msg`

### 2. Regenerate Go proto code
- Run `protoc` from `core-backend/` → `tts.pb.go`, `tts_grpc.pb.go`

### 3. Regenerate Python proto code
- Run `grpc_tools.protoc` from `core-tts/` → `tts_pb2.py`, `tts_pb2_grpc.py`

### 4. Rewrite `core-tts/grpc_server.py`
- Unary `Synthesize` method dispatching on `request.engine`
- `asyncio.run()` for fast mode (same thread-safe pattern as current code)
- 50MB message limits

### 5. Modify `core-tts/main.py`
- Start gRPC server when `GRPC_PORT` env var is set
- Runs in background threads via grpc's ThreadPoolExecutor

### 6. Rewrite `core-backend/client/grpc_tts.go`
- Unary `Synthesize` method matching `CoreTTSClient.Synthesize` signature
- 50MB max receive message size
- `WithBlock()` dial with 5s timeout

### 7. Modify `core-backend/client/core_tts.go`
- Add `grpcClient *GrpcTTSClient` field to `CoreTTSClient`
- Add `SetGrpcClient()` method
- In `Synthesize()`: dispatch to gRPC when `grpcClient != nil`, else HTTP
- Add `Close()` method for graceful shutdown

### 8. Modify `core-backend/app/bootstrap.go`
- After creating `CoreTTSClient`, if `CoreTTSGrpcURL` is set, create `GrpcTTSClient` and wire it in
- Log warning on gRPC failure, continue with HTTP

### 9. Modify `docker-compose.yml`
- Add `GRPC_PORT=50051` to `core-engine` environment

### 10. Update `core-backend/config/config.go` logSummary
- Show gRPC status in startup log

## Files Modified
| File | Action |
|------|--------|
| `core-backend/proto/tts.proto` | Rewrite |
| `core-tts/proto/tts.proto` | Rewrite (same content) |
| `core-backend/proto/tts.pb.go` | Regenerate |
| `core-backend/proto/tts_grpc.pb.go` | Regenerate |
| `core-tts/proto/tts_pb2.py` | Regenerate |
| `core-tts/proto/tts_pb2_grpc.py` | Regenerate |
| `core-tts/grpc_server.py` | Rewrite |
| `core-tts/main.py` | Modify (add gRPC startup) |
| `core-backend/client/grpc_tts.go` | Rewrite |
| `core-backend/client/core_tts.go` | Modify (add gRPC dispatch) |
| `core-backend/app/bootstrap.go` | Modify (wire gRPC client) |
| `docker-compose.yml` | Modify (add GRPC_PORT) |
| `core-backend/config/config.go` | Modify (logSummary) |
