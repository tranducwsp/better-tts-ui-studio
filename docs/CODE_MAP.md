# Bản đồ mã nguồn — dùng để review

Mục tiêu của tệp này: mở đúng tệp cần đọc mà không phải lần theo import. Không mô tả tệp
kiểm thử. Thứ tự trình bày theo đường đi của một request, không theo thứ tự thư mục.

Ba dịch vụ:

```
frontend (Svelte 5)  ──HTTP──>  core-backend (Go, :8000)  ──HTTP──>  core-tts (Python, :8001)
   giao diện                     xác thực, hạn mức, lịch sử          suy luận AI, không auth
                                        │
                                   PostgreSQL + Redis
```

Ranh giới tin cậy: **chỉ core-backend là biên phòng vệ.** core-tts không có xác thực và
tin mọi thứ nó nhận; nó chỉ được bảo vệ bằng việc không mở cổng ra host. Mọi kiểm tra đầu
vào có ý nghĩa an ninh đều phải nằm ở core-backend.

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
| 1 | `core-backend/main.go` | Thứ tự khởi động: cái gì phải có trước khi cổng mở |
| 2 | `core-backend/router/router.go` | Toàn bộ danh sách route và middleware của từng route — bản đồ bề mặt tấn công |
| 3 | `core-backend/middleware/auth.go` | Danh tính được xác lập thế nào, và ba mức bảo vệ khác nhau ra sao |
| 4 | `core-backend/state/manifest.go` | Mọi hạn mức được thi hành ở đây |
| 5 | `core-backend/handlers/unified.go` | Handler tổng hợp — nơi 4 tệp trên gặp nhau |

---

## core-backend (Go) — biên phòng vệ

### Khởi động và cấu hình

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `main.go` | 156 | Thứ tự khởi động, và **cổng gate**: manifest phải nạp được trước khi listener mở. Chứa `manifestDiscoveryTimeout` (90s) — con số vận hành đáng chất vấn nhất trong repo. |
| `config/settings.go` | 194 | **Bảng đặc tả mọi biến môi trường**: tên, mặc định, khoảng hợp lệ, tài liệu. `.env.example` được sinh ra từ đây. Muốn biết biến X làm gì thì đọc đúng một chỗ này. |
| `config/config.go` | 217 | Đọc bảng trên thành struct `Config`. `requireAll()` chặn khởi động khi thiếu biến bắt buộc. `logSummary()` in cấu hình đang có hiệu lực. |
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
| `handlers/tasks.go` | 314 | Trạng thái task, huỷ task, tải audio (kèm chuyển mã), và **SSE** stream tiến độ. `ownsTask` kiểm quyền cho cả 4 endpoint. `safeTaskID` chặn traversal. |
| `handlers/utils.go` | 271 | Bóc văn bản từ tệp tài liệu (.txt/.pdf/.docx/.odt). DOCX và ODT là zip → **đây là chỗ còn lỗ hổng zip bomb chưa vá** (dòng ~118 và ~175). |
| `handlers/auth.go` | 273 | Đăng ký, đăng nhập (đặt cookie HttpOnly), đăng xuất, `/me`, và hai API admin (liệt kê, duyệt user). |
| `handlers/history.go` | 295 | Lịch sử job và chi tiết chunk. `GetJobDetail` là ví dụ tốt về kiểm quyền sở hữu đúng cách. |
| `handlers/engine_sync.go` | 84 | `POST /api/internal/engine/reload`: nạp lại Manifest và bắn webhook rebuild sang frontend-builder. Route admin. |
| `handlers/health.go` | 49 | `/health`, `/ready`, và `/api/info` (trả Manifest từ RAM). |
| `handlers/respond.go` | 37 | `writeJSON`, `writeError`, `currentUser`. Đọc trước các handler khác — 4 hàm ngắn dùng ở khắp nơi. |

### Trạng thái, lưu trữ, dữ liệu

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `state/tasks.go` | 524 | Task bất đồng bộ: tiến độ, hủy, pub/sub cho SSE (fanout nội bộ + Redis khi có nhiều replica), cache audio. Có `GlobalTaskManager` — một trong hai singleton của repo. Tệp phức tạp nhất về đồng thời. |
| `db/db.go` | 228 | Khởi tạo pool pgx, chạy migration (golang-migrate, embed), tạo tài khoản mặc định. `RegisterJobAndChunk` và `UpdateChunkStatus` là hai hàm ghi được handler dùng. |
| `db/schema.sql` | 52 | 4 bảng: `users`, `user_voices`, `tts_jobs`, `tts_chunks` + index. Đọc để hiểu quan hệ `chunk → job → user` (nền tảng của kiểm quyền task). |
| `db/query/*.sql` | 17 truy vấn | Nguồn thật của mọi câu SQL. `sqlc` sinh code Go từ đây → **không sửa `db/sqlc/` bằng tay**, sửa ở đây rồi chạy `sqlc generate`. |
| `db/sqlc/*.go` | 708 | **Sinh tự động.** Bỏ qua khi review, trừ khi đang kiểm chính bản sinh. |
| `storage/paths.go` | 51 | Dựng mọi đường dẫn lưu trữ (`TempDir`, `ModeDir`) từ một gốc duy nhất, kèm `safeSegment` làm sạch. Nhỏ, đáng đọc trọn. |
| `storage/sweeper.go` | 81 | Dọn tệp audio tạm quá hạn, định kỳ và một lần lúc khởi động. |
| `audio/transcode.go` | 120 | Chuyển mã bằng ffmpeg qua pipe (không tệp tạm), có hàng đợi giới hạn theo số nhân CPU và timeout. Tham số ffmpeg là bảng cố định, không ghép từ đầu vào. |

### Giao tiếp với Engine

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `client/core_tts.go` | 200 | Client HTTP tới core-tts: `GetInfo` (manifest), `GetVoices`, `Synthesize`, `CloneVoice`. Đường đi thật đang dùng. |
| `client/grpc_tts.go` | 136 | Client gRPC streaming. **Đã viết, chưa nối — để dành cho sau.** Bỏ qua ở lần review này; xem ghi chú cuối tệp. |
| `proto/*.pb.go` | 601 | **Sinh tự động** từ `.proto`, phục vụ đường gRPC ở trên. Bỏ qua khi review. |

---

## core-tts (Python, FastAPI) — engine suy luận

Không có xác thực. Mọi endpoint mở cho bất kỳ ai gọi tới được, nên nó **phải** nằm trong
mạng nội bộ.

| Tệp | Dòng | Chức năng |
|-----|------|-----------|
| `schemas.py` | 283 | **Định nghĩa Manifest phía Python** — phải khớp `core-backend/types/manifest.go`. Đây là hợp đồng giữa hai dịch vụ; lệch ở đây là lệch toàn hệ thống. |
| `routes.py` | 176 | Mọi endpoint: `/info`, `/voices`, `/synthesize`, `/tasks/{id}`, `/voices/clone`, `/health`. |
| `engine.py` | 152 | Suy luận thật: nạp model vieneu (lười), giọng Edge TTS cho mode `fast`, và `get_preset_voices` lọc giọng theo mode. |
| `main.py` | 46 | Dựng app FastAPI, CORS, và điểm khởi động uvicorn. |
| `config.py` | 20 | Giới hạn luồng CPU/GPU, host/port, thư mục lưu trữ. |
| `grpc_server.py` | 76 | Server gRPC streaming. **Đã viết, chưa nối** — `main.py` chưa gọi `serve_grpc()`. Cùng nhóm để dành với `client/grpc_tts.go`. |

`core-tts-test/` là **mock service** (không có torch/edge-tts, trả 2 tệp audio tĩnh) dùng để
chạy frontend không cần GPU. Tự khai `engine_name="Mock Test AI Engine"` trong manifest.
Không nằm trong `docker-compose.yml`; muốn dùng phải tự đổi build context.

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
| `.env.example` | **Sinh tự động** từ `config/settings.go`. Đừng sửa tay; chạy `cd core-backend && go generate ./config`. |
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

**3. Đường gRPC đã viết nhưng chưa nối — có chủ đích, không phải rác.** `client/grpc_tts.go`
(136 dòng) chưa được tệp nào ngoài chính nó tham chiếu; `core-tts/grpc_server.py` chưa được
`main.py` gọi; `CORE_ENGINE_GRPC_URL` được đọc vào `Config` rồi chưa ai dùng. Mọi lượt tổng
hợp hiện đi qua HTTP trong `client/core_tts.go`.

Đây là phần **để dành cho sau**, sẽ nối vào khi đường HTTP đã review xong và ổn định — nên
đừng xoá, và cũng đừng tính nó vào phạm vi review lần này. Ghi ở đây chỉ để bạn không mất thời
gian truy tại sao `StreamStandardSynthesize` không có ai gọi, và để không ai kết luận rằng
streaming hiện đang chạy trên gRPC (nó chạy trên SSE, xem `handlers/tasks.go`).
