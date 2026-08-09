# Bản đồ mã nguồn — dùng để review

Mục tiêu của tệp này: mở đúng tệp cần đọc mà không phải lần theo import. Không mô tả tệp
kiểm thử. Thứ tự trình bày theo đường đi của một request, không theo thứ tự thư mục.

Ba dịch vụ:

```
frontend (Svelte 5)  ──HTTP──>  backend (Go, :8000)  ──HTTP/gRPC──>  engine (external, :8001)
   giao diện                     xác thực, hạn mức, lịch sử                suy luận AI, không auth
                                        │
                                   PostgreSQL + Redis
```

Ranh giới tin cậy: **chỉ backend là biên phòng vệ.** Engine không có xác thực và
tin mọi thứ nó nhận; nó chỉ được bảo vệ bằng việc không mở cổng ra host. Mọi kiểm tra đầu
vào có ý nghĩa an ninh đều phải nằm ở backend.

Điểm cần nắm trước khi đọc bất cứ thứ gì: **Manifest là nguồn của mọi hạn mức.** Engine tự
khai nó chấp nhận gì (độ dài văn bản, khoảng speed/pitch, danh sách mode, định dạng tệp), và
nền tảng thi hành theo bản khai đó. Nên `state/manifest.go` và `types/manifest.go` là hai tệp
chi phối hành vi của phần lớn các tệp còn lại.

---

## Bắt đầu từ đâu

Nếu chỉ có thời gian cho một đường đọc, đi theo thứ tự này — đây là đường đi của một lượt
tổng hợp giọng nói, và nó chạm vào mọi tầng:

| # | Tệp | Vì sao đọc ở bước này |
|---|-----|----------------------|
| 1 | `backend/cmd/web/main.go` | Entrypoint HTTP: nạp config, bootstrap web, mở server |
| 2 | `backend/router/router.go` | Toàn bộ danh sách route và middleware của từng route — bản đồ bề mặt tấn công |
| 3 | `backend/middleware/auth.go` | Danh tính được xác lập thế nào, và ba mức bảo vệ khác nhau ra sao |
| 4 | `backend/state/manifest.go` | Mọi hạn mức được thi hành ở đây |
| 5 | `backend/handlers/unified.go` | Handler tổng hợp — nơi 4 tệp trên gặp nhau |

---

## backend (Go) — biên phòng vệ

### Khởi động và cấu hình

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `cmd/web/main.go` | 12 | Entrypoint web: config → `app.BootstrapWeb` → `web.Run`. |
| `cmd/worker/main.go` | 12 | Entrypoint worker: config → `app.BootstrapWorker` → `worker.Run`. |
| `cmd/cron/main.go` | 13 | Entrypoint cron: config → `app.BootstrapCron` → `cron.Run`. |
| `app/bootstrap.go` | 110 | Ba bootstrap rõ nghĩa: web chạy migration/seed/auth và cấu hình advanced policy; worker chạy DB/Redis/Engine; cron chỉ khởi tạo storage rồi trả kho cho `cron.Run`. Manifest gate dùng cho web và worker. |
| `web/server.go` | 78 | `http.Server` cùng bốn hạn thời gian của nó, và trình tự tắt gọn. |
| `worker/worker.go` | 84 | Vòng lặp nhặt job: trần `maxInFlight` mỗi tiến trình, và `wg.Wait()` để job dở dang chạy nốt khi nhận tín hiệu dừng. |
| `cron/sweeper.go` | 56 | Vòng lặp dọn âm thanh tạm: chạy sweep ngay lúc khởi động và mỗi giờ; không mở cổng, không nối DB/Redis/Engine. Giữ đúng 1 replica. |
| `config/settings.go` | 283 | **Bảng đặc tả mọi biến môi trường**: tên, mặc định, khoảng hợp lệ, tài liệu. Nhóm `Advanced deployment tuning` có default an toàn và không bắt AI engineer phải đổi. `.env.example` được sinh ra từ đây. Muốn biết biến X làm gì thì đọc đúng một chỗ này. |
| `config/config.go` | 265 | Đọc bảng trên thành struct `Config`. `requireAll()` chặn khởi động khi thiếu biến bắt buộc; các advanced knobs có range validation và default. `logSummary()` in cấu hình đang có hiệu lực. |
| `cmd/gen-env/main.go` | 91 | Sinh `.env.example` từ `config/settings.go`. Chạy bằng `go generate ./config`. |

### Hợp đồng với Engine (Manifest)

Hai tệp này quyết định hành vi của phần lớn hệ thống. Đọc `types/manifest.go` trước.

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `types/manifest.go` | 356 | **Định nghĩa toàn bộ cấu trúc Manifest.** Quan trọng nhất là 4 hàm giải nghĩa: `ResolveCapabilities` và `ResolveAudioSpec` (mode khai → engine khai → mặc định nền tảng), `ChunkSize()`, `TextLimit()`. Chú ý: hai hàm Resolve *lặng lẽ* trả mặc định khi gặp mode lạ — nên chúng **không dùng được để kiểm tính hợp lệ**. |
| `types/validate.go` | 150 | Kiểm một Manifest có tự mâu thuẫn không, lúc nạp. Tách **lỗi** (từ chối: không có mode, mode trùng id) khỏi **cảnh báo** (nhận nhưng ghi log). |
| `state/manifest.go` | 199 | Giữ Manifest trong RAM (có `sync.RWMutex`) và **thi hành hạn mức**: `ValidateRequest` (độ dài text, speed, mode), `ValidatePitch`, `ValidateEmotion`, `HasMode`. Cả ba hàm Validate đều fail-closed khi chưa có manifest. |

### Xác thực và phân quyền

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `security/auth.go` | 59 | Nguyên thuỷ mật mã: bcrypt hash/verify, tạo và kiểm JWT (HS256). Nhỏ và đáng đọc trọn vẹn. |
| `middleware/auth.go` | 131 | `AuthMiddleware` đọc token từ cookie hoặc header rồi nạp user vào context — **nó không chặn ai**. Việc chặn thuộc ba middleware riêng: `RequireAuth` (đã đăng nhập), `RequireActiveUser` (+ đã được admin duyệt), `RequireAdmin` (+ role admin). Phân biệt ba cái này là chìa khoá đọc `router.go`. |
| `middleware/user_cache.go` | 99 | Cache user ngắn hạn để mỗi request không phải SELECT. `InvalidateUser` phải được gọi ở mọi nơi đổi role hoặc trạng thái duyệt. |
| `middleware/ratelimit.go` | 134 | Giới hạn nhịp theo IP, cửa sổ trượt. Chỉ áp cho `/login` và `/register`. Bộ đếm trong RAM mỗi tiến trình. **Tin `X-Forwarded-For` vô điều kiện** — xem phần cảnh báo cuối tệp này. |
| `router/router.go` | 120 | **Bản đồ mọi route và middleware bảo vệ nó.** Tệp quan trọng nhất để nắm bề mặt tấn công: một route nằm sai nhóm là một lỗ hổng. Đọc sau `middleware/auth.go`. |

### Handlers (theo mức độ đáng soi)

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `handlers/unified.go` | 302 | **Cổng tổng hợp chính.** `GetVoices` và `Synthesize`. Chạy chuỗi validate theo Manifest, tạo task, ghi job/chunk, rồi sinh audio trong goroutine nền. Đường đi nóng nhất của hệ thống. |
| `handlers/upload.go` | 165 | Nhận tệp multipart: trần kích thước, allowlist định dạng theo Manifest, **kiểm magic bytes** (bắt tệp bị đổi phần mở rộng), và kiểm `model_id` trước khi nó vào đường dẫn tệp. |
| `handlers/tts_clone.go` | 333 | Nhân bản giọng: lưu tệp tham chiếu, gọi engine trích embedding, ghi DB. Có `DeleteUserVoice`. Nơi đường dẫn tệp được dựng từ đầu vào người dùng. |
| `handlers/tasks.go` | 401 | Trạng thái task, huỷ task, tải audio (kèm chuyển mã), và **SSE** stream tiến độ. `ownsTask` kiểm quyền cho cả 4 endpoint. `safeTaskID` chặn traversal. Presigned URL TTL lấy từ advanced deployment config. |
| `handlers/utils.go` | 271 | Bóc văn bản từ tệp tài liệu (.txt/.pdf/.docx/.odt). DOCX và ODT là zip → **đây là chỗ còn lỗ hổng zip bomb chưa vá** (dòng ~118 và ~175). |
| `handlers/auth.go` | 350 | Đăng ký, đăng nhập (access + refresh cookie HttpOnly), refresh access token chỉ từ refresh cookie, đăng xuất, `/me`, và hai API admin. |
| `handlers/history.go` | 295 | Lịch sử job và chi tiết chunk. `GetJobDetail` là ví dụ tốt về kiểm quyền sở hữu đúng cách. |
| `handlers/engine_sync.go` | 84 | `POST /api/internal/engine/reload`: nạp lại Manifest và bắn webhook rebuild sang frontend-builder. Route admin. |
| `handlers/health.go` | 49 | `/health`, `/ready`, và `/api/info` (trả Manifest từ RAM). |
| `handlers/respond.go` | 37 | `writeJSON`, `writeError`, `currentUser`. Đọc trước các handler khác — 4 hàm ngắn dùng ở khắp nơi. |

### Trạng thái, lưu trữ, dữ liệu

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `state/task_item.go` | 204 | Dữ liệu và hành vi của một task: owner, trạng thái, audio trong RAM, cancel và snapshot. |
| `state/task_manager.go` | 311 | Map task trong RAM, đồng bộ với Redis, quyền sở hữu, cache transcode và cleanup. |
| `state/task_pubsub.go` | 172 | Redis Pub/Sub và fanout SSE cho TaskItem; gồm generation guard để tránh subscription cũ phát trùng. |
| `state/redis_state.go` | 126 | Redis client singleton và trạng thái online của user. |
| `state/manifest.go` | 199 | Manifest singleton và các hàm validate runtime. |
| `db/db.go` | 289 | Khởi tạo pool pgx; migration và tài khoản mặc định chạy theo `InitOptions` (chỉ web, để nhiều tiến trình không giành cùng một khoá migration). `RegisterJobAndChunk` và `UpdateChunkStatus` là hai hàm ghi được handler dùng — hàm đầu cũng là nơi kiểm job có thuộc người gọi không. |
| `db/schema.sql` | 52 | 4 bảng: `users`, `user_voices`, `tts_jobs`, `tts_chunks` + index. Đọc để hiểu quan hệ `chunk → job → user` (nền tảng của kiểm quyền task). |
| `db/query/*.sql` | 17 truy vấn | Nguồn thật của mọi câu SQL. `sqlc` sinh code Go từ đây → **không sửa `db/sqlc/` bằng tay**, sửa ở đây rồi chạy `sqlc generate`. |
| `db/sqlc/*.go` | 708 | **Sinh tự động.** Bỏ qua khi review, trừ khi đang kiểm chính bản sinh. |
| `storage/paths.go` | 51 | Dựng mọi đường dẫn lưu trữ (`TempDir`, `ModeDir`) từ một gốc duy nhất, kèm `safeSegment` làm sạch. Nhỏ, đáng đọc trọn. |
| `storage/sweeper.go` | 40 | `SweepTempObjects`: thao tác dữ liệu thuần — liệt kê nhánh temp và xoá tập quá hạn. Vòng lặp chạy định kỳ nằm ở `cron.Run`, không phải ở đây. |
| `audio/transcode.go` | 118 | Chuyển mã bằng ffmpeg qua pipe (không tệp tạm), có concurrency/timeout lấy từ advanced platform config. Tham số ffmpeg là bảng cố định, không ghép từ đầu vào. |

### Hàng đợi và tổng hợp

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `queue/queue.go` | 146 | Hàng đợi job trên Redis Stream: `Enqueue`, `EnsureGroup`, `Consume`. **Cố ý không bền** — không AOF/RDB, không `XAUTOCLAIM`: một chunk chỉ tốn vài giây để chạy lại và frontend đã tự retry, nên mất job là hành vi đã chọn chứ không phải sơ suất. |
| `synth/synth.go` | 129 | Một lượt tổng hợp từ đầu tới cuối: gọi Engine → ghi kho → cập nhật DB → phát trạng thái. Dùng chung giữa worker và nhánh dự phòng của web, nên hai đường không thể trôi khỏi nhau. |
| `synth/local.go` | 55 | Nhánh dự phòng khi không có Redis: chạy tổng hợp ngay trong tiến trình web, có ghi nhận để lúc tắt máy còn chờ chạy nốt. |

### Giao tiếp với Engine

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `client/core_tts.go` | 200 | Client HTTP tới engine: `GetInfo` (manifest), `GetVoices`, `Synthesize`, `CloneVoice`. Đường đi thật đang dùng. |
| `client/grpc_tts.go` | 136 | Client gRPC streaming. **Đã viết, đã nối — đường streaming dùng gRPC.** |
| `proto/*.pb.go` | 601 | **Sinh tự động** từ `.proto`, phục vụ đường gRPC ở trên. Bỏ qua khi review. |

---

## engine (external) — engine suy luận

Engine là repo riêng của AI engineer, không nằm trong repo này. Nó không có xác thực, mọi
endpoint mở cho bất kỳ ai gọi tới được, nên nó **phải** nằm trong mạng nội bộ.

Hợp đồng giữa backend và engine được quy định ở `backend/types/manifest.go` và
`engine/schemas.py` (bên repo engine). Lệch ở đây là lệch toàn hệ thống.

`core-tts-example/` là **example engine** (không cần GPU, sinh sóng sin ngẫu nhiên) dùng để
chạy frontend mà không cần model thật. Tự khai `engine_name="Mock Test AI Engine"` trong manifest.
README ghi rõ contract (endpoints, manifest, format) mà nền tảng mong đợi — AI engineer dùng
làm bản tham khảo rồi tự thay bằng engine thật. Không nằm trong `docker-compose.yml`;
muốn dùng phải tự đổi build context.

---

## frontend (Svelte 5 + TypeScript)

Điểm cần biết: giao diện **được dựng từ Manifest** — nó không viết cứng mode, giọng hay hạn
mức nào. Các tệp trong `lib/` là bản song song của logic giải nghĩa Manifest ở backend, nên
chúng phải cho cùng kết quả (có fixture dùng chung ở `docs/capability-resolution-cases.json`).

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `src/lib/api.ts` | 305 | Mọi lệnh gọi HTTP tới backend, tập trung một chỗ. Đọc trước tiên ở phần frontend. |
| `src/lib/types.ts` | 217 | Kiểu TypeScript của Manifest — bản song song của `types/manifest.go`. |
| `src/lib/capabilities.ts` | 105 | Giải nghĩa capability theo mode. Song song với `ResolveCapabilities` trong Go. |
| `src/lib/audioSpec.ts` | 231 | Giải nghĩa định dạng audio: nhận gì khi upload, tải về được gì, MIME type. Song song với `ResolveAudioSpec`. |
| `src/lib/textLimits.ts` | 161 | Trần độ dài, kích thước chunk, ngưỡng chuyển sang streaming, và hàm cắt văn bản. Song song với `TextLimit`/`ChunkSize`. |
| `src/lib/ranges.ts` | 66 | Khoảng speed/pitch cho slider. |
| `src/lib/audioWav.ts` | 63 | Đóng gói `AudioBuffer` thành WAV (dùng khi cắt tệp tham chiếu ở client). |
| `src/lib/toast.svelte.ts` | 21 | Trạng thái thông báo (Svelte 5 runes). |
| `src/App.svelte` | 314 | Thành phần gốc: nạp manifest, điều phối trạng thái toàn cục. |
| `src/main.ts` | 24 | Điểm gắn ứng dụng vào DOM. |

Thành phần giao diện (`src/lib/components/`), theo độ lớn:

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `GenericEnginePanel.svelte` | 494 | Bảng điều khiển sinh động theo Manifest cho mode thường. |
| `StreamingPanel.svelte` | 458 | Bảng cho mode streaming, tiêu thụ SSE. |
| `TextInputPanel.svelte` | 441 | Nhập văn bản, xem trước cách cắt chunk, tải tệp tài liệu. |
| `WaveformTrimmer.svelte` | 223 | Cắt audio tham chiếu bằng Web Audio API. |
| `CreateVoiceModal.svelte` | 207 | Tạo giọng nhân bản, các trường sinh từ `voice_metadata_schema`. |
| `VoiceSelect.svelte` | 157 | Chọn giọng, lọc theo mode. |
| `HistoryModal.svelte` | 141 | Lịch sử tổng hợp. |
| `AuthModal.svelte` | 114 | Đăng nhập / đăng ký. |
| `AdminModal.svelte` | 103 | Quản trị user (duyệt tài khoản). |
| `Header.svelte` | 83 | Thanh đầu trang. |
| `Toast.svelte` | 12 | Hiển thị thông báo. |

`frontend/scripts/builder_server.js` là sidecar Node: nhận webhook `POST /rebuild` rồi chạy
`npm run build` để dựng lại HTML tĩnh. Không có xác thực (xem cảnh báo cuối).

---

## Triển khai

| Tệp | Chức năng |
|-----|-----------|
| `docker-compose.yml` | 6 dịch vụ. **Chỉ 5173 (UI) và 8000 (API) mở ra host**; postgres, redis, engine, builder chỉ dùng `expose` trong mạng nội bộ. Bắt buộc `SECRET_KEY`, `POSTGRES_PASSWORD`, `REDIS_PASSWORD` qua cú pháp `:?`. |
| `.env.example` | **Sinh tự động** từ `config/settings.go`. Đừng sửa tay; chạy `cd backend && go generate ./config`. |
| `k8s/` | Helm chart (deployment, ingress, service, pvc, sealed secret). `auth-sealedsecret.yaml` chứa ciphertext — commit là đúng thiết kế. |
| `.gitea/workflows/ci.yml` | Hiện **comment toàn bộ** — không có gate CI nào chạy. |
| `docs/` | Đặc tả protocol, hướng dẫn tích hợp engine, tham chiếu cấu hình, và `capability-resolution-cases.json` (fixture dùng chung cho test Go và frontend). |

---

## Ba điều nên biết trước khi review

**1. `handlers/utils.go` còn một lỗ hổng chưa vá.** DOCX/ODT là zip; `io.ReadAll` ở dòng ~118
và ~175 không có trần, nên tệp nén 597 KB giải nén thành 600 MB (đã đo, ~1000x).
`MaxBytesReader` chỉ chặn kích thước tệp nén nên không cản được. Route có yêu cầu đăng nhập.

**2. `middleware/ratelimit.go` tin `X-Forwarded-For` vô điều kiện.** Nếu backend không đứng
sau proxy tin cậy, ai cũng vượt được hạn mức bằng cách đổi header mỗi request — tức lớp chống
dò mật khẩu coi như không có. Nên chỉ đọc header này khi có cấu hình xác nhận đứng sau proxy.

**3. Đường gRPC đã được nối.** `client/grpc_tts.go` được dùng cho đường streaming,
`CORE_ENGINE_GRPC_URL` cấu hình địa chỉ gRPC endpoint. Mọi lượt tổng hợp
streaming đi qua gRPC trong `client/grpc_tts.go`; đường HTTP trong `client/core_tts.go`
vẫn dùng cho các yêu cầu không streaming.
