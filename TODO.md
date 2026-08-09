# TODO — Review toàn diện better-tts-ui-studio

> Sinh từ đợt review (BE Go, FE Svelte/TS, core-tts Python, manifest/env ergonomics, deploy).
> Tick `[x]` khi xử lý xong. Độ nghiêm trọng: 🔴 CRITICAL · 🟠 HIGH · 🟡 MEDIUM · 🟢 LOW.

## A. Manifest & env cho AI engineer (mục nhấn mạnh)

- [x] **[CRITICAL] A1 — Docs hứa `ENABLE_AUTH` / shared mode không tồn tại** ✅ ĐÃ XỬ LÝ (2026-08-08)
  - `README.md:18-20`, `docs/ENGINEER_INTEGRATION_GUIDE.md:145-157`
  - Quyết định: **auth là bắt buộc** — gỡ **hẳn** toàn bộ phần hứa hẹn zero-login/shared mode khỏi tài liệu:
    - `README.md`: bỏ bullet "Dual Authentication & Storage Modes", thay bằng câu chỉ có Authenticated mode; sửa dòng diagram "Shared Mode Router" → "Per-User Isolation".
    - `docs/ENGINEER_INTEGRATION_GUIDE.md`: bỏ khỏi TOC + gỡ hẳn section về mode (không thay bằng section khác), cuối tài liệu đi thẳng tới congratulations.
  - Shared mode (zero-login cho demo/team nội bộ) là **feature làm sau** — xem mục F1.
  - Không update `core-backend/config`: vốn không tồn tại biến `ENABLE_AUTH` trong Settings table; không cần sửa code.

---

## F. Future / làm sau

- [ ] **[FEATURE] F1 — Shared mode (zero-login, dùng chung nội bộ)**
  - Đã gỡ hứa hẹn khỏi docs (A1). Khi làm lại: cần thêm knobs cấu hình thật (`ENABLE_AUTH` hoặc tương đương) vào `config.Settings`, route `/storage/{model_id}/shared/`, và chính sách cô lập dữ liệu rõ ràng.
- [x] **[HIGH] [Done 2026-08-09] A2 — `.env.example` thiếu toàn bộ biến runtime của engine**
  - **Ranh giới nền tảng**: chỉ cần `CORE_ENGINE_URL` để biết địa chỉ engine. Biến nội bộ của engine (`OMP_NUM_THREADS`, `UI_MODE`, `CORE_CORS_ORIGINS`, v.v.) là việc của AI engineer — nền tảng không áp đặt. `core-tts-test` đã viết lại thành bản example engine, không còn là "mock load test" — README ghi rõ contract, AI engineer tự quyết định biến môi trường riêng.
- [ ] **[HIGH] A3 — `PRESERVE_FILES`: env không tài liệu đổi hành vi xoá dữ liệu**
  - `core-backend/handlers/tts_clone.go:344`
  - `DeleteUserVoice` chỉ xoá file khi `PRESERVE_FILES` không set. Không có trong Settings table, `.env.example`, hay doc → rò rỉ audio giọng tham chiếu vĩnh viễn.
  - Fix: đưa vào Settings table + `.env.example`, hoặc bỏ đọc env và chuyển retention thành config table-backed.
- [ ] **[MEDIUM] A4 — `input_panel` thiếu field → default `false`, mâu thuẫn engine**
  - `core-backend/types/manifest.go:111-116`, `frontend/src/lib/components/TextInputPanel.svelte:290,331,336,386,417`, `core-tts/schemas.py:150-156`
  - Khác `capabilities`/`audio_spec` (resolve mode→engine→platform), `InputPanelSpec` bool omit → Go `false`/TS falsy. Fallback "all true" chỉ khi cả block vắng. Engine emit `input_panel` thiếu `replace_tool`/`enable_chunk_box` → mất công cụ upload.
  - Fix: thêm cơ chế resolve như capabilities, hoặc document rõ hành vi omit.
- [ ] **[MEDIUM] A5 — Dev FE local chống backend host hỏng ngay khi cài**
  - `frontend/vite.config.ts:4` — proxy target mặc định `http://core-backend:8000` (DNS chỉ tồn tại trong compose). `npm run dev` trên host không hoạt động trừ khi `VITE_BACKEND_URL=http://localhost:8000`.
  - Fix: doc section local-dev; thêm CORS/origin doc cho FE serve riêng.
- [ ] **[LOW] A6 — Trùng header group trong `.env.example`**
  - `.env.example:96,130` (2× `── Storage ──`), `:141,169` (2× `── CORS ──`).
  - Fix: đổi tên group trong `settings.go` (`Storage (limits)` / `CORS (cookie)`...).

## B. Engine core-tts (Python)

- [ ] **[CRITICAL] B1 — Neural inference chạy sync trên event loop → một synthesis đóng băng cả service**
  - `core-tts/routes.py:98,162`
  - `synthesize_standard_sync`/`encode_reference` (CPU-bound) gọi trực tiếp trong `async def`, không `asyncio.to_thread`. `/health`, `/info`, `/voices` và mọi fast-mode request treo trong lúc synthesis chạy.
  - Fix: `asyncio.to_thread(...)`; load model nền khi khởi động.
- [ ] **[HIGH] B2 — Cancel là no-op; "cancelled" → "done" khi chạy xong**
  - `core-tts/routes.py:141-147` set `cancel=True` nhưng `engine.py:91-121` không check; `routes.py:86-87,100-101` ghi đè `status="done"`. Backend Go cancel HTTP nhưng engine bỏ qua disconnect.
  - Fix: check `cancel` trong vòng lặp tổng hợp; (phối hợp backend `synth/pipeline.go:127-131`).
- [ ] **[HIGH] B3 — `supports_voice_saving=True` nhưng voice clone chỉ sống trong RAM**
  - `core-tts/schemas.py:245-247`, `engine.py:36` (`cloned_voices_cache`), `routes.py:149,164-168`
  - Restart engine / nhiều uvicorn worker → backend PostgreSQL trỏ `voice_id` engine không còn biết → 500.
  - Fix: hoặc gỡ `supports_voice_saving=True`, hoặc persist voice ra file/storage.
- [ ] **[HIGH] B4 — "Voice B (Male)" tổng hợp ra giọng nữ**
  - `core-tts/engine.py:123-132` fallback `vi-VN-HoaiMyNeural` (nữ) cho tên không chứa "nam"/"minh". `schemas.py:202-204` bán "Voice B (Male)".
  - Fix: sửa `get_fast_voice_code` map đúng giọng nam; thống nhất namespace id giữa registry và preset.
- [ ] **[HIGH] B5 — Không enforce biên giới đã khai: text/upload vô hạn**
  - `routes.py:55-113` không validate `len(text)`/`speed`; `routes.py:153` `file.read()` không cap. Manifest khai `max_text_length=3000`, `max_upload_bytes=200MB`.
  - Fix: validate ở `/synthesize` và `clone_voice` theo đúng manifest.
- [ ] **[HIGH] B6 — Arbitrary file read/write qua param**
  - Đọc: `engine.py:106-109` (`os.path.exists(voice)` → mở file bất kỳ). Ghi: `routes.py:90-92,103-107` build path từ `req.engine` + `req.task_id` (caller-controlled).
  - Fix: loại bỏ đường mở file theo string; validate `engine`/`task_id` bằng allowlist + sanitize path.
- [ ] **[MEDIUM] B7 — File temp và RAM audio không bao giờ được giải phóng**
  - `routes.py:93-94,106-107`; `cleanup_tasks_db` (`engine.py:38-42`) chỉ dọn dict. Đã tích tụ file orphan (vd `storage/temp/clone_3223b850.wav` Aug 6).
  - Fix: sweep file temp theo tuổi (phối hợp `core-backend/cron/sweeper.go` hoặc sweep riêng của engine).
- [ ] **[MEDIUM] B8 — `STORAGE_DIR` bị bỏ qua; ghi đĩa trước khi trả response**
  - `config.py:19-20` define nhưng `routes.py:90,103` ghi cứng relative `"storage"` → phụ thuộc CWD; read-only/disk-full container → mọi synthesis 500 dù audio đã sinh xong.
  - Fix: dùng `STORAGE_DIR` thật; ghi sau khi có thể trả, hoặc ghi vào storage backend.
- [ ] **[LOW] B9 — Manifest/behavior mismatch**
  - `supports_streaming=True` (`schemas.py:264`) nhưng không stream; `output_format`/`pitch`/`ref_voice_id` nhận nhưng không dùng; docstring `/synthesize` hứa "Async Task ID" không có; `/health` báo `vram_used_mb: 0` giả.
  - Fix: bỏ field khai mà không implement, hoặc implement; `/health` trả metric thật.

## C. Backend Go

- [x] **[HIGH] C1 — Cancel job đang chờ queue bị mất; job vẫn chạy và thành `done`**
  - Nguyên nhân (3 chỗ): `GetOrCreate` dựng `TaskItem` mới `Cancel=false` và ghi đè lên trạng thái `cancel:true` vừa lưu trên Redis; `WatchCancel` subscribe **sau** khi check cờ đầu tiên — Redis pub/sub không phát lại nên cancel tới sớm bị mất; `CancelTask` chỉ set cờ RAM + publish, không ai đưa chunk còn `pending`/`processing` trong `tts_chunks` về `cancelled`.
  - Đã sửa:
    - `GetOrCreate` (`state/task_manager.go`) đọc trạng thái Redis **trước** khi dựng bản mới; `applyRemoteState` giờ copy `Cancel`.
    - `WatchCancel` (`state/task_item.go`) đọc lại khoá `task:<id>` **ngay sau khi subscribe** — đóng khe publish-bị-mất.
    - `Notify` (`state/task_pubsub.go`) đảo thứ tự pipeline: `Set` trạng thái **trước** `Publish` để "nếu bản tin mất thì bản ghi đã có".
    - `CancelTask` (`handlers/tasks.go`) ghi `tts_chunks.status='cancelled'` tại thời điểm huỷ qua `db.CancelChunk` (query `CancelTTSChunk` trong `db/query/chunks.sql`), guard theo `status` để không đè kết quả `done`/`error`.
  - Test: `core-backend/state/task_cancel_test.go` — 3 test (cancel trước khi worker nhận; cancel trong khoảng subscribe; cancel khi đã nghe). Chạy ngược với mã cũ: test 1, 2 **FAIL** (đúng), test 3 PASS. Toàn bộ `go test ./... -race` **PASS** với Redis tạm (`TEST_REDIS_ADDR`).
- [x] **[HIGH] C2 — Refresh session không rotation, không revoke, sống 30 ngày; logout cosmetic**
  - Đã fix (2026-08-08): bảng `auth_sessions` (jti = PK) + rotation trong 1 tx; reuse → 401 "already used" (KHÔNG hạ family — nhiều tab chung cookie jar tự hồi phục); logout thu hồi phía DB; phiên hết hạn TUYỆT ĐỐI (rotation không kéo dài); sweep phiên quá hạn opportunistic tại login (cron không có DB). Refresh & logout scope `Path=/api/auth`; logout tại `/api/auth/logout` để cookie tới được; frontend retry-once 401 trong `refreshSession`.
  - Test: `refresh_token_test.go` (jti round-trip, exp khớp phiên) + curl E2E: login → refresh rotate → reuse 401 → logout → reuse "revoked". `/api/auth/refresh` trong nhóm rate limit RIÊNG scope `refresh` (khoá Redis `ratelimit:refresh:<ip>`), tách khỏi login/register scope `auth` — frontend gọi refresh trên mỗi lần tải trang nên gộp chung sẽ làm cạn budget login của chính người dùng.
- [x] **[MEDIUM] C3 — Endpoint JSON không cap body size**
  - Đã fix (2026-08-09): middleware `middleware.BodyLimit` (2 MiB) bọc toàn bộ `/api`, bỏ qua multipart (upload tự có `MaxBytesReader` theo `MAX_UPLOAD_SIZE_MB`). Body quá trần bị ngắt ngay ở tầng đọc; trước đây Sonic buffer vô hạn → RAM tăng tuyến tính.
  - Test: `tests/bodylimit_test.go` — JSON 3MiB bị 413, multipart 5MiB đi qua, body nhỏ đi qua.
- [x] **[MEDIUM] C4 — Multipart parse buffer toàn bộ 256 MiB trong RAM rồi copy lần 2**
  - Đã fix (2026-08-09): `parseUpload` gọi `ParseMultipartForm(32MB)` thay vì truyền trần upload làm maxMemory → file lớn hơn 32MB spill ra tệp tạm trên đĩa, không ở RAM. `receiveReferenceAudio` chỉ đọc 512 byte đầu để kiểm chữ ký rồi trả `multipart.File` (io.Seeker) thay vì `io.ReadAll` toàn bộ. `Store.Put` và `CloneVoice` đổi sang nhận `io.Reader` (stream qua io.Pipe), không còn `bytes.Buffer`/`[]byte` cho cả tệp. Thêm `cleanupMultipartForm` (RemoveAll) để dọn tệp tạm spill ra đĩa. Test: `go test ./...` pass; E2E curl upload-temp + upload persistent (storage+engine) đều 200.
- [x] **[HIGH] C5 — Redis restart → mọi job pending/in-flight kẹt `processing` vĩnh viễn**
  - Đã fix (2026-08-09): **SQL reconcile trong cron** (file riêng `cron/reconciler.go`, không chung `sweeper.go`). Không chọn XAUTOCLAIM vì stream Redis non-persistent — Redis restart là mất sạch entry, không còn gì để claim; XAUTOCLAIM chỉ cứu "worker chết mà Redis sống" (kịch bản phụ).
  - `tts_chunks` chưa từng có cột thời gian → thêm migration `000011` cột `updated_at`, `UpdateTTSChunkStatus`/`CancelTTSChunk` dời nó về `now()`.
  - Query `ReconcileStaleChunks` (`db/query/chunks.sql`): chunk còn `pending`/`processing` mà `updated_at` cũ hơn mốc → `error` kèm message "Job mất sau khi Redis khởi động lại".
  - Cron: `BootstrapCron` giờ mở DB (chỉ ghi/đọc, không migrate/seed); `cmd/cron/main.go` truyền `STALE_CHUNK_AFTER_MINUTES` (setting mới, default 30, `gen-env` đã regenerate `.env.example`); vòng Reconcile 5 phút + một lượt lúc khởi động, DB lỗi thì bỏ qua lượt không chết tiến trình; compose `core-cron` thêm `DATABASE_URL` + `depends_on: postgres`.
  - Test: `go test ./...` pass; E2E — chèn chunk `processing` cũ 2h + chunk `processing` mới, restart cron → chunk cũ thành `error` (`Job mất sau khi Redis khởi động lại...`), chunk mới giữ nguyên.
- [x] **[MEDIUM] C6 — `/extract-text` ngốn ~½GB+/request**
  - Đã fix (2026-08-09): **bóp RAM bự nhất ra khỏi handler + cột chặn ở router**.
  - `handlers/utils.go`: input đọc có trần (`maxExtractBytes=32MiB`, `.txt` chỉ lấy tới trần output), output cắt ở `maxExtractOutputBytes` (200KiB), PDF regex giới hạn số match (`maxPdfMatches`) và bỏ `string(data)` copy — fallback quét thẳng `[]byte` với `sb.Grow` có trần; DOCX/ODT giải nén qua `readZipEntry` (64MiB).
  - Router: `/extract-text` nằm trong nhóm riêng 2 lớp bảo vệ: `ConcurrencyLimit(8)` (chặn tổng RAM ~không phụ thuộc input) + `RateLimitUser("extract", 20, 10m)` (khoá theo user ID, fallback IP); `middleware.ratelimit.go` refactor thành `rateLimitBy(keyFn, ...)` dùng chung cho `RateLimit`/`RateLimitUser`, sửa cả lỗi tàn dư `ip` trong `reap`.
  - Test: `go test -race ./tests/` pass (`TestRateLimitUser_PerUser/FallsBackToIP`, `TestConcurrencyLimit_CapsActive`, `TestExtractText_ThroughRouter/RequiresAuth`); `go vet` pass. E2E live Docker: unauth 401, auth 200, bắn 26 request liên tiếp → đúng 20 lượt 200 còn 7 cái 429.
- [x] **[MEDIUM] C7 — History cap 200, không pagination**
  - Đã fix (2026-08-09): **pagination theo con trỏ (keyset) — không OFFSET**.
  - `db/query/jobs.sql`: `ListUserHistorySummaries` thêm `before_id`/`before_created_at` (`sqlc.arg(...)`, nullable = trang đầu), predicate so `(created_at, id) < (before, before_id)` đúng thứ tự `ORDER BY created_at DESC, id DESC` để chỉ mục chạy; sqlc sinh `BeforeCreatedAt pgtype.Timestamptz` + `BeforeID`.
  - `handlers/history.go`: đọc `before` (RFC3339, sai → 400) + `before_id`; hỏi `historyPageSize+1` rồi tính `has_more`; trả envelope `{items, has_more}`; mỗi item thêm `created_at` định dạng đủ microsecond (round-trip làm cursor không lệch thứ tự khi hai job cùng giây). Admin history dùng chung hàm nên tự được hưởng.
  - Frontend: `types.ts` thêm `created_at`/`HistoryPage`; `api.ts` `fetchHistory(before?)` + `HistoryCursor`; `HistoryModal.svelte` thêm nút "Load more" nối trang sau (dedup theo job_id), dùng chung cho tab admin.
  - Test: `go test ./...` pass (kể cả `TestHistory_GetUserHistory_BeforeNotTimestamp` 400); `go vet` + `gofmt` clean; `npm run check` clean. E2E live Docker: 3 job tạo thật → con trỏ ở vị trí oldest/middle/newest cho trang sau rỗng / chỉ job cũ nhất / đúng 2 job theo thứ tự cũ-trước; admin `/admin/users/{id}/history` cùng hành vi; bundle đã build lại chứa "Load more".
- [x] **[LOW] [Done 2026-08-09] C8 — bcrypt 72-byte cap → 500 lỗi**
  - `security/auth.go`: thêm `MaxPasswordBytes=72`, `minPasswordChars=8`, `ValidatePassword`; `HashPassword` yêu cầu validate trước.
  - `handlers/auth.go`: `Register` 400 kèm message rõ khi password quá ngắn/dài; `Login` về 401 đồng nhất khi >72 byte. Test: `TestAuth_Register_PasswordLength` (7 ký tự / 80 byte → cả hai 400).
- [x] **[LOW] [Done 2026-08-09] C9 — `schema.sql` và migration lệch default `tts_chunks.status`**
  - Migration mới `000012` đặt `DEFAULT 'pending'` (down: `'processing'`); `schema.sql` là nguồn thật. `tests/status_default_parity_test.go` mô phỏng chuỗi migration (last-write-wins) để so với schema — chặn lệch tái phát, chạy không cần Postgres.
- [x] **[MEDIUM] [Done 2026-08-09] C10 — `GetVoices` gọi engine mỗi request, không cache**
  - `handlers/voice_cache.go`: cache theo mode (key chuẩn hoá "" → "all"), hai lớp hết hạn — `voiceCacheTTL=30s` và version manifest thay đổi (`currentManifestVersion()`); miss mới gọi `GetVoices` rồi lưu.
  - `handlers/unified.go`: đọc cache trước, chỉ fetch khi miss; bản lỗi không lấp cache.
  - Test: `TestPresetVoiceCache_Hit` (key chuẩn hoá), `_ManifestChangeInvalidates`, `_ExpiresByTTL`.

## D. Frontend (Svelte/TS)

- [x] **[CRITICAL] [Done 2026-08-09] D1 — Vòng lặp generation chạy tiếp sau khi component unmount**
  - `StreamingPanel.svelte`: thêm `onMount` cleanup — đặt `isCancelled = true` + revoke blob URLs khi component unmount. Đóng panel sau 5/50 chunk → 45 chunk còn lại dừng ngay, không tốn GPU, không rò blob URL.
- [x] **[HIGH] [Done 2026-08-09] D2 — Auto-format trên blur phá huỷ ký tự không phải tiếng Việt**
  - `TextInputPanel.svelte`: bỏ `onblur={() => runAutoFormat(false)}`. Auto-format chỉ chạy khi bấm nút "Clean Text" hoặc upload file — không còn tự động xoá emoji, CJK, em-dash khi người dùng rời ô.
- [x] **[HIGH] [Done 2026-08-09] D3 — Hardcode `?format=wav` → file `.mp3` chứa byte WAV**
  - `api.ts`: `subscribeTaskStream` thêm tham số `format` (default `'wav'`), dùng `encodeURIComponent`. `StreamingPanel` truyền `defaultFmt` (từ manifest) thay vì hardcode.
- [x] **[MEDIUM] [Done 2026-08-09] D4 — Không refresh token cho request ngoài `checkCurrentUser`**
  - `api.ts`: thêm `authFetch()` — wrapper tự thử `refreshSession()` một lần khi gặp 401, thay cho 12 chỗ `fetch` trần. `checkCurrentUser` đơn giản hoá nhờ wrapper. Các public endpoint (login, register, info, refresh) giữ `fetch` gốc.
- [x] **[MEDIUM] [Done 2026-08-09] D5 — Race khi đổi tab voices**
  - `GenericEnginePanel.svelte`: thêm bộ đếm `voiceLoadGeneration` — mỗi lần gọi `loadVoicesForMode` tăng generation, fetch xong kiểm tra generation còn đúng không. Mode cũ về sau bị bỏ, không ghi đè.
- [x] **[MEDIUM] [Done 2026-08-09] D6 — SSE rớt mạng → restart nguyên chunk từ đầu**
  - `api.ts`: `onerror` không đóng EventSource ngay — kiểm tra `readyState`: CONNECTING (browser đang reconnect) thì chờ, CLOSED (lỗi vĩnh viễn 404/403) mới gọi onError. Backend SSE gửi snapshot trạng thái khi mở lại, nên không mất tiến độ. Trước đây onerror đóng ngay → retry tạo task mới, vứt 90% đã xong.
- [x] **[BUG] [Done prior] D7 — `loginUser` trả `data.user || data` nhưng backend không có field `user`**
  - Backend đã thêm field `user` vào login response (C5: `handlers/auth.go:228-234`). Frontend `data.user` trả đúng UserResponse.
- [x] **[LOW] [Done 2026-08-09] D8 — History dedup key gồm `time_ago`**
  - C7 đã thay dedup key bằng `job_id` — `time_ago` không còn trong key.
- [x] **[LOW] [Done 2026-08-09] D9 — Clone reference không resample về 24kHz**
  - `audioWav.ts`: thêm `resampleAudioBuffer()` dùng `OfflineAudioContext` (Web Audio API, không thư viện ngoài). `audioSpec.ts`: thêm `defaultSampleRate()`. `WaveformTrimmer.svelte`: `handleTrimOnly` resample về engine default trước khi encode.

## E. Deploy / k8s / CI

- [ ] **[CRITICAL] E1 — Chart k8s chỉ render backend; không thể deploy**
  - `k8s/templates/deployment.yaml`, `values.yaml:26-52` — không Postgres/Redis/engine/worker/cron/frontend. Backend Pod chết sau 90s (`app/bootstrap.go:72-77`), CrashLoopBackOff; Svelte không được serve.
  - Fix: thêm template cho toàn bộ thành phần, hoặc gỡ chart và ghi rõ k8s unsupported.
- [ ] **[CRITICAL] E2 — Backend pod không có env DB/engine/storage; `DATABASE_URL` mặc định placeholder `<change>`**
  - `k8s/templates/deployment.yaml:51-53` (`envFrom` secret chỉ 2 key), `settings.go:104`, `.env.example:53`.
  - Fix: env đầy đủ qua secret/configmap; bỏ placeholder (hoặc `DATABASE_URL` Required).
- [ ] **[HIGH] E3 — README quick start làm hỏng auth trên http://localhost**
  - `README.md:61-67`, `.env.example:146` (`COOKIE_SECURE=1`), `handlers/auth.go:201,206,245,263` — cookie `Secure` trên http → browser không lưu → mọi request auth 401.
  - Fix: README phải nói `COOKIE_SECURE=0` cho http (hoặc compose default 0 + document HTTPS flip).
- [ ] **[MEDIUM] E4 — `docker-compose.s3-pin.yml` chỉ patch `core-backend`**
  - `docker-compose.s3-pin.yml:17-20` — `core-worker`/`core-cron` gọi SDK qua hostname hỏng → `SignatureDoesNotMatch`.
  - Fix: patch `extra_hosts` cho cả `core-worker` và `core-cron`.
- [ ] **[MEDIUM] E5 — Traefik basicAuth middleware chết, trỏ nhầm secret**
  - `k8s/templates/auth-middleware.yaml:11-12` trỏ `studio-auth-secret` (chứa `SECRET_KEY`/`ACCESS_TOKEN_EXPIRE_MINUTES`, không có `users`); ingress không tham chiếu middleware.
  - Fix: secret `users` riêng + annotation trên ingress, hoặc xoá CRD chết.
- [ ] **[MEDIUM] E6 — Frontend nginx proxy hardcode `container_name` compose; port lệch**
  - `frontend/nginx.conf:18,27` (`ai-core-backend:8000`), `values.yaml:44-52` (`frontend.service.port: 80` vs image serve 5173).
  - Fix: parametrize upstream (env/service name); sửa port 5173; thêm frontend deployment+ingress.
- [ ] **[MEDIUM] E7 — Base image không pin, `npm install` thay vì `npm ci`**
  - `core-backend/Dockerfile:2,16` (`golang:alpine`, `alpine:latest`), `core-tts/Dockerfile:1`, `frontend/Dockerfile*:1-2`, `frontend/Dockerfile:8`.
  - Fix: pin digest + `npm ci` + lockfile.
- [ ] **[LOW-MED] E8 — Không `/metrics`; `/ready` chỉ ping DB; Redis fail fallback âm thầm**
  - `handlers/health.go:20-35`, `state/redis_state.go:60-63` — multi-replica vẫn "Ready" khi mất consistency cross-process.
  - Fix: `/metrics`; đưa Redis vào readiness (hoặc log rõ fallback).
- [ ] **[LOW] E9 — CI bị comment toàn bộ; test parity không chạy**
  - `.gitea/workflows/ci.yml` — `TestEnvExampleIsUpToDate`, Go/TS parity, `svelte-check` không chạy ở đâu. Image CI `vieneu-tts-api` không khớp `harbor.amoratran.id.vn/tiny-src/*`.
  - Fix: bật lại CI (gen-env/parity + svelte-check) + `docker compose up` smoke test.

---

## G. Thứ tự sửa đề xuất (top 12)

1. B1 — engine: đưa inference sang thread + load model nền.
2. B2 — engine: check cancel flag giữa các bước (backend-side C1 đã xong; còn khoảng cách engine đến khi TTS client trả).
3. B5/B6 + C3/C4 — validate text/upload, chặn path traversal, cap body, stream ra storage.
4. A2/A3 (A1 đã xử lý) — thêm biến engine vào `.env.example`; `PRESERVE_FILES` vào Settings.
5. B3 — gỡ `supports_voice_saving=True` hoặc persist voice thật.
6. C2 — ✅ đã xong (2026-08-08): refresh rotation + `jti` + revoke + rate-limit refresh route.
7. D1/D2/D3 — cleanup unmount; bỏ auto-format blur; `?format=` theo defaultFmt.
8. C5 + C6 — ✅ đã xong (2026-08-09): SQL reconcile trong cron (`cron/reconciler.go`); `/extract-text` giảm RAM + concurrency/rate-limit theo user.
9. D7 — sửa `loginUser` trả user object.
10. E1/E2/E3 — k8s chart đầy đủ (hoặc bỏ); env pod; `COOKIE_SECURE=0` trong README.
11. B4 — `get_fast_voice_code` giọng nam đúng.
12. E9 — bật CI parity + smoke test.
