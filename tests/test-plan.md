# Kế hoạch Test API — Mọi luồng tương tác & hệ thống

> Tài liệu này liệt kê **tất cả** luồng hoạt động của platform qua API, không qua giao diện.
> Mục đích: chuẩn bị test case trước khi chạy thực tế.

---

## 1. Authentication & Session

### 1.1 Đăng ký (Register)

| Test case | Input | Expected |
|-----------|-------|----------|
| Đăng ký thành công | `POST /api/register` với `{username, password}` hợp lệ | `201` + `user.id`, `is_approved: false` |
| Username đã tồn tại | `POST /api/register` với username đã có | `400` + `"Username already exists"` |
| Password quá ngắn | `POST /api/register` với password < 8 ký tự | `400` + `"password must be at least 8 characters"` |
| Username rỗng | `POST /api/register` với `username: ""` | `400` |
| Body thiếu field | `POST /api/register` với `{}` | `400` |
| Rate limit vượt | Gọi `/api/register` 10+ lần / phút | `429` + `"rate limit exceeded"` |

### 1.2 Đăng nhập (Login)

| Test case | Input | Expected |
|-----------|-------|----------|
| Đăng nhập thành công | `POST /api/login` với đúng user/pass | `200` + `access_token`, `refresh_token` cookie |
| Sai password | `POST /api/login` với sai password | `401` + `"invalid credentials"` |
| User chưa approved | `POST /api/login` với user chưa được duyệt | `403` + `"pending approval"` |
| User không tồn tại | `POST /api/login` với username không có | `401` |
| Rate limit vượt | Gọi `/api/login` 10+ lần / phút | `429` |

### 1.3 Refresh token

| Test case | Input | Expected |
|-----------|-------|----------|
| Refresh thành công | `POST /api/auth/refresh` với cookie `refresh_token` hợp lệ | `200` + `access_token` mới |
| Refresh token hết hạn | `POST /api/auth/refresh` với cookie cũ > 30 ngày | `401` + `"token expired"` |
| Không có cookie | `POST /api/auth/refresh` không gửi cookie | `401` |
| Refresh token đã revoked | `POST /api/auth/refresh` với token đã logout | `401` + `"session revoked"` |

### 1.4 Logout

| Test case | Input | Expected |
|-----------|-------|----------|
| Logout thành công | `POST /api/auth/logout` với cookie `refresh_token` | `200` + `"logged out"` |
| Session bị revoked | Sau logout, gọi `/api/auth/refresh` với token cũ | `401` |
| Logout không có token | `POST /api/auth/logout` không gửi cookie | `200` (idempotent — xóa cookie phía client) |

### 1.5 Lấy thông tin user hiện tại

| Test case | Input | Expected |
|-----------|-------|----------|
| Get me thành công | `GET /api/me` với `Authorization: Bearer <token>` | `200` + `{username, role, is_approved}` |
| Token hết hạn | `GET /api/me` với token cũ > 15 phút | `401` |
| Không gửi token | `GET /api/me` không header | `401` |
| Token sai format | `GET /api/me` với `Bearer invalid` | `401` |

---

## 2. Manifest & Engine Info

### 2.1 Lấy manifest

| Test case | Input | Expected |
|-----------|-------|----------|
| Get info thành công | `GET /api/info` | `200` + `UniversalManifest` đầy đủ 9 trường |
| Manifest chưa load | Khi engine chưa kết nối | `503` + `Service Unavailable` |
| Kiểm tra field chết | Response có `supports_ssml` không? | Không có |
| Kiểm tra field chết | `input_panel` có `closeable` không? | Không có |
| Kiểm tra đủ field | `capabilities` có đúng 7 field? | Đúng |
| Kiểm tra đủ field | `input_panel` có đúng 5 field? | Đúng |

### 2.2 Reload manifest (admin)

| Test case | Input | Expected |
|-----------|-------|----------|
| Reload thành công | `POST /api/internal/engine/reload` (admin) | `200` + manifest mới |
| Không phải admin | Gọi với user role `user` | `403` |
| Engine chết | Engine không phản hồi → reload | `502` + `Bad Gateway` |

---

## 3. Synthesis — Tổng hợp giọng nói

### 3.1 Synthesize — Happy path & input validation

| Test case | Input | Expected |
|-----------|-------|----------|
| Synthesize thành công | `POST /api/synthesize/{model_id}` với text ngắn | `200` + `task_id`, `status: queued` |
| Text quá dài | Input > `max_text_length` (3000) | `400` + `"text too long"` |
| Text rỗng | Body `{"text": ""}` | `400` |
| Text chỉ whitespace | Body `{"text": "   \n   "}` | `400` |
| Text không gửi | Body `{}` | `400` |
| Text với special chars | Unicode, emoji, null byte, control chars | Vẫn synthesize hoặc báo lỗi hợp lệ |
| Text với ký tự rất dài (ko khoảng trắng) | Chuỗi 2000 ký tự liền, ko space | Worker phải chunk hoặc xử lý được |
| Text đúng bằng max_text_length | Text đúng 3000 ký tự | `200` + queued |
| Model không tồn tại | `POST /api/synthesize/no_such_model` | `400` + `"engine mode is not supported"` |
| Model_id không phù hợp | `zero_shot_clone` mà ko gửi reference audio | Lỗi hợp lệ |
| Voice_id không hợp lệ | `voice_id` không tồn tại trong preset | `400` |
| Speed ngoài range | `speed: 0.0` hoặc `speed: 3.0` | `400` |

### 3.2 Synthesize — Auth & permission

| Test case | Input | Expected |
|-----------|-------|----------|
| Thiếu authentication | Không gửi token | `401` |
| Token hết hạn | Gửi token cũ > 15 phút | `401` |
| Token sai format | `Authorization: Bearer invalid` | `401` |
| Token sai chữ ký | Token tự ký giả | `401` |
| Unapproved user | User chưa được duyệt | `400` |
| User bị de-approve đang có task chạy | Admin approve → unapprove khi task đang xử lý | Task vẫn chạy? API từ chối request mới |
| User bị xóa đang có task chạy | Admin xóa user khi task đang xử lý | Task vẫn chạy? Hay failed? |

### 3.3 Synthesize — Rate limit & concurrency

| Test case | Input | Expected |
|-----------|-------|----------|
| Rate limit synthesize | Gọi `/api/synthesize` 20+ lần / phút | `429` |
| Rate limit riêng biệt từng user | User A gọi 20 lần, User B gọi 20 lần | User A bị 429, User B vẫn 200 |
| Concurrency limit | Gửi 21+ task cùng lúc (ko stream) | `429` + `"too many concurrent"` |
| Vượt concurrency → sau đó giảm tải | Gửi 21 task → 429 → đợi 1 task xong → gửi lại | Request thành công |
| Rate limit reset sau 1 phút | Gọi 21 lần → 429 → đợi 1 phút → gọi tiếp | `200` |

### 3.4 Synthesize — Redis failure

| Test case | Hành vi mong đợi |
|-----------|-----------------|
| Redis down trước khi synthesize | Queue push fail → `503` + `"service unavailable"` |
| Redis down trước khi synthesize (rate limit) | Rate limit counter không đọc được → fallback cho phép hoặc từ chối? |
| Redis down mid-synthesis | Task state không save được → worker không cập nhật được status |
| Redis down sau khi synthesize | Task queue gửi được → worker không đọc được task state |
| Redis restart khi task đang chạy | Task state mất → task không poll được (404) |
| Redis memory full (eviction) | Key cũ bị evict → task state mất, audio mất |
| Redis key bị corruption | Task state không deserialize được → worker bỏ qua hoặc báo lỗi |
| Redis rất chậm (high latency) | Timeout khi push queue → synthesize fail chậm |
| Redis network partition | Rate limit ko hoạt động → abuse được, user cache miss → fallback DB |
| Redis password sai | `AUTH` fail → không connect được → 503 |
| Redis maxmemory policy sai | Key bị evict sớm hơn TTL dự kiến |

### 3.5 Synthesize — PostgreSQL failure

| Test case | Hành vi mong đợi |
|-----------|-----------------|
| PG down trước khi synthesize | User validation (cache miss → query DB) → `500` |
| PG down sau khi complete | Task hoàn thành → history không insert được → audio vẫn có nhưng ko có history |
| PG connection pool full | Slow request → timeout → `500` |
| PG replication lag | User vừa approved → cache miss → query replica chưa kịp update → thấy user chưa approved |
| PG restart | Kết nối bị drop → query fail → retry? |
| PG slow query (history insert) | Insert history chậm → response chậm |
| PG unique constraint violation | Race condition → duplicate history entry? |

### 3.6 Synthesize — Engine failure

| Test case | Hành vi mong đợi |
|-----------|-----------------|
| Engine crash | Task queue không consume được → task stuck "queued" → timeout → failed |
| Engine crash khi đang xử lý | Worker gRPC call fail → task `status: error` + reason |
| Engine hang (không response) | Worker đợi → timeout → task failed |
| Engine trả về lỗi HTTP/gRPC | Worker nhận error code → task `status: error` + error message |
| Engine trả về audio rỗng (0 byte) | Worker nhận 0 byte → task failed |
| Engine trả về audio corrupted | Worker nhận data → save → user tải về file hỏng |
| Engine trả về sai format (nói WAV nhưng gửi MP3) | Content-Type mismatch → UI ko phát được |
| Engine trả về sai sample rate | Audio nghe bị méo tiếng hoặc tốc độ sai |
| Engine gửi chunk đầu nhưng die giữa chừng (stream) | Stream bị cắt ngang → task failed |
| Engine mất kết nối gRPC với backend | `502 Bad Gateway` |
| Engine gửi manifest mới khi đang synthesize | Không ảnh hưởng task đang chạy? |
| Engine không support model_id được gửi | `400` hoặc `404` |
| Engine xử lý quá chậm (> 5 phút) | Redis TTL expire → task state mất → user thấy 404 |
| Engine gửi progress update sai (0% → 200%) | UI progress bar sai |
| Engine memory leak (OOM) | Engine die → task failed |
| Engine GPU out of memory | Task failed với error "CUDA OOM" |

### 3.7 Streaming synthesis — Chi tiết

| Test case | Input | Expected |
|-----------|-------|----------|
| Streaming thành công | `POST /api/synthesize/{model_id}` với `stream: true` | `200` + SSE stream |
| Text ngắn (1 chunk) | Text < chunk_size | 1 audio chunk + `event: completed` |
| Text dài (nhiều chunk) | Text gấp 3 lần chunk_size | Nhiều audio chunk + progress events |
| Chunk đầu tiên nhanh | Text dài → chunk đầu gửi ngay | User nghe được phần đầu khi phần sau đang render |
| Streaming + speed != 1.0 | `stream: true` + `speed: 1.5` | Audio chunk với speed đã áp dụng |
| Streaming + voice_id | `stream: true` + `voice_id: hoai_my` | Audio chunk với giọng đã chọn |
| Streaming + emotion | `stream: true` + `emotion: happy` | Audio chunk với emotion (nếu mode support) |
| Client ngắt kết nối giữa chừng | Fetch API abort → cancel | Task bị cancel, worker cleanup |
| Client ngắt kết nối sau khi hoàn thành | Fetch API abort sau `event: completed` | Không ảnh hưởng |
| Network error giữa stream | TCP connection reset | Stream failed, task failed |
| Engine gửi chunk sai thứ tự | Chunk 2 đến trước chunk 1 | Backend vẫn forward đúng thứ tự |
| Engine gửi progress event sai | Progress 50% → 10% | Bỏ qua hoặc xử lý? |
| Streaming token hết hạn giữa chừng | Token expiry < stream duration | Stream vẫn tiếp tục đến hết hoặc bị cắt? |
| Streaming với concurrent limit | 20 stream cùng lúc | 21st bị 429 |

### 3.8 Theo dõi task (poll)

| Test case | Input | Expected |
|-----------|-------|----------|
| Task đang chạy | `GET /api/tasks/{task_id}` | `200` + `status: processing` |
| Task hoàn thành | `GET /api/tasks/{task_id}` sau khi xong | `200` + `status: done` |
| Task lỗi | `GET /api/tasks/{task_id}` khi engine lỗi | `200` + `status: error` + `reason` |
| Task bị cancel | `GET /api/tasks/{task_id}` sau khi cancel | `200` + `status: cancelled` |
| Task không tồn tại | `GET /api/tasks/nonexistent` | `404` |
| Task của user khác | Dùng token user A, get task user B | `403` |
| Task state bị corrupt | Redis key bị ghi đè | `500` hoặc parse error |
| Poll task quá nhanh (polling 10ms) | Gọi API liên tục mỗi 10ms | Rate limit? |
| Poll task sau khi TTL expire | Task > 5 phút | `404` |
| Task state kiểu "queued" | Chưa có worker nhận | `200` + `status: queued` |
| Task state transition: queued → processing → done | Poll 3 lần | Mỗi lần trả về status đúng |

### 3.9 Hủy task

| Test case | Input | Expected |
|-----------|-------|----------|
| Cancel thành công | `POST /api/tasks/{task_id}/cancel` | `200` + `status: cancelled` |
| Cancel task đã done | `POST /api/tasks/{task_id}/cancel` với done task | `400` + `"Task is already done"` |
| Cancel task đã error | `POST /api/tasks/{task_id}/cancel` với error task | `400` + `"Task is already error"` |
| Cancel task đã cancelled | `POST /api/tasks/{task_id}/cancel` 2 lần | `400` + `"Task is already cancelled"` |
| Cancel task không tồn tại | `POST /api/tasks/nonexistent/cancel` | `404` |
| Cancel không phải chủ | Dùng token user khác | `403` |
| Cancel task đang processing | Cancel khi worker đang xử lý | Task cancelled, worker cleanup |
| Cancel task đang queued | Cancel trước khi worker nhận | Task removed khỏi queue |
| Cancel và Poll ngay sau đó | Cancel → GET task | `200` + `status: cancelled` |
| 2 request cancel cùng lúc | Gửi 2 cancel đồng thời | Idempotent — cả 2 đều 200 hoặc 1 cái 400 |
| Cancel khi Redis down | Redis không update được state | `503` |
| Cancel khi engine đang chết | Worker không nhận được cancel signal | Task marked cancelled nhưng engine vẫn chạy |

### 3.10 Lấy audio

| Test case | Input | Expected |
|-----------|-------|----------|
| Audio tồn tại | `GET /api/tasks/{task_id}/audio` | `200` + audio binary |
| Audio chưa sẵn sàng | Task chưa done | `404` + `"Audio is not ready"` |
| Audio hết hạn (TTL) | Task > 5 phút | `404` |
| Audio hết hạn giữa lúc download | Download bắt đầu, TTL expire giữa chừng | Stream vẫn tiếp tục? Hay bị cắt? |
| Audio của user khác | Dùng token user A, get audio task B | `403` |
| Audio không tồn tại (task failed) | Task failed, ko có audio | `404` |
| Content-Type đúng | Kiểm tra header | `audio/wav` hoặc `audio/mpeg` |
| Content-Length đúng | Kiểm tra header | Khớp với kích thước binary |
| Range request (partial content) | `Range: bytes=0-1023` | `206 Partial Content` nếu server support |
| Audio file WAV | Task output WAV | Binary đúng WAV header |
| Audio file MP3 | Task output MP3 | Binary đúng MP3 header |
| Audio file lớn (> 10MB) | Task dài → audio lớn | Download thành công, ko timeout |
| Audio 0 byte | Task thành công nhưng engine trả về 0 byte | `200` + file 0 byte? Hay lỗi? |
| Audio download + Redis down | Redis ko check được state | Fallback? Hay 503? |

### 3.11 Streaming task progress (SSE)

| Test case | Input | Expected |
|-----------|-------|----------|
| SSE stream task | `GET /api/stream/tasks/{task_id}` | `200` + `text/event-stream` |
| Progress events: queued → processing | Task tiến triển | `data: {"status":"processing","progress":0}` |
| Progress events: processing → done | Task hoàn thành | `data: {"status":"done","progress":100}` |
| Progress events: processing → error | Task lỗi | `data: {"status":"error","progress":0}` |
| Progress events: processing → cancelled | Task bị cancel | `data: {"status":"cancelled","progress":0}` |
| Event cuối | `data: {"status":"done"}` | `data: {status, progress}` — đúng spec SSE |
| Task không tồn tại | `GET /api/stream/tasks/nonexistent` | `404` |
| Client ngắt kết nối SSE | Client close connection | Backend cleanup, ko leak goroutine |
| SSE reconnect | Client đứt → reconnect với cùng task_id | Server gửi lại state hiện tại |
| Task hoàn thành trước khi SSE mở | Task done → GET stream | `data: {"status":"done"}` ngay lập tức |
| Nhiều client SSE cùng task | 2 client mở stream cùng 1 task_id | Cả 2 đều nhận event |
| SSE timeout | Server idle timeout > 30s | Connection đóng, client reconnect |
| SSE + Redis down | Redis ko publish được event | Worker không thể notify → client timeout |

### 3.12 Task queue & Worker — Chi tiết

| Test case | Hành vi mong đợi |
|-----------|-----------------|
| Queue rỗng | Worker BRPOP block → chờ task mới, ko tốn CPU |
| 1 task trong queue | Worker nhận task → process → complete |
| Nhiều task trong queue | Worker xử lý FIFO (theo thứ tự) |
| Task queue ưu tiên | Task A gửi sau → xử lý trước nếu có priority? |
| Worker crash khi đang xử lý | Task stuck "processing" → timeout → failed |
| Worker restart | Worker nhận task mới từ queue, task cũ stuck |
| Task timeout (> 5 phút) | Worker ko complete kịp → Redis TTL expire → 404 |
| Queue message bị corrupt | Worker nhận message ko parse được → bỏ qua hoặc log error |
| Queue message bị duplicate | Worker nhận 2 message giống nhau → process 2 lần? |
| Queue backlog (1000+ task) | Task ở cuối queue chờ rất lâu → TTL expire trước khi xử lý |
| Worker process chậm (memory leak) | Worker eat memory → OOM kill → task stuck |
| Worker pool có 2 worker | 2 task cùng lúc → mỗi worker 1 task |
| Queue bị xóa (flush) | Tất cả task pending mất → ko ai biết |
| Publish task vào queue fail | Redis down → ko push được → 503 |

### 3.13 Task state machine — Các transition đặc biệt

| Test case | Transition | Hành vi mong đợi |
|-----------|-----------|-----------------|
| queued → processing | Worker nhận task | Task state update trong Redis |
| queued → cancelled | User cancel trước khi xử lý | Task removed khỏi queue |
| processing → done | Worker hoàn thành | Audio save, history insert |
| processing → failed | Engine lỗi | Worker ghi lỗi, ko save audio |
| processing → cancelled | User cancel giữa chừng | Worker cleanup, ko save audio |
| done → cancelled | User cancel sau khi xong | `400` — ko hợp lệ |
| failed → retry | Worker retry? | Có retry policy ko? |
| done → audio fetched | User download | Audio vẫn còn trong TTL |
| done → audio expired | TTL > 5 phút | 404 |
| queued → (worker crash) → queued? | Worker crash → task vẫn trong queue | Task vẫn chờ? Hay mất? |

---

## 4. Voice — Giọng nói

### 4.1 Lấy preset voices

| Test case | Input | Expected |
|-----------|-------|----------|
| Lấy voice theo model | `GET /api/voices/{model_id}` | `200` + array voice |
| Model không tồn tại | `GET /api/voices/no_such_model` | `200` + `[]` (engine trả về danh sách rỗng) |
| Engine không có voice | Model không có preset_voices | `200` + `[]` |

### 4.2 Upload voice (clone)

| Test case | Input | Expected |
|-----------|-------|----------|
| Upload thành công | `POST /api/clone/upload` với file audio | `200` + `voice_id` |
| File quá lớn | File > `max_upload_bytes` | `413` + `"file too large"` |
| Sai format | File không phải audio hợp lệ | `400` + `"invalid format"` |
| Không gửi file | `POST /api/clone/upload` với body rỗng | `400` |
| Chưa login | Không gửi token | `401` |
| Upload temp | `POST /api/clone/upload-temp` | `200` + temp `voice_id` |

### 4.3 Quản lý user voices

| Test case | Input | Expected |
|-----------|-------|----------|
| Lấy voice của user | `GET /api/clone/voices` | `200` + array voice |
| User không có voice | User mới, chưa upload | `200` + `[]` |
| Xóa voice | `DELETE /api/clone/voices/{clone_id}` | `200` + `"deleted"` |
| Xóa voice không tồn tại | `DELETE /api/clone/voices/nonexistent` | `404` |
| Xóa voice của user khác | Dùng token user A, xóa voice user B | `403` |

---

## 5. History — Lịch sử

### 5.1 Lấy lịch sử

| Test case | Input | Expected |
|-----------|-------|----------|
| History có dữ liệu | `GET /api/history` | `200` + `{items, has_more}` |
| History trống | User mới, chưa synthesize | `200` + `items: []` |
| Phân trang | `GET /api/history?cursor=...` | `200` + page tiếp theo |
| Pagination hết | `GET /api/history?cursor=last` | `200` + `has_more: false` |

### 5.2 Lấy job detail

| Test case | Input | Expected |
|-----------|-------|----------|
| Job tồn tại | `GET /api/history/{job_id}` | `200` + `{job_id, engine, voice, ...}` |
| Job không tồn tại | `GET /api/history/nonexistent` | `404` |
| Job của user khác | Dùng token user A, get job user B | `403` |

### 5.3 Init job

| Test case | Input | Expected |
|-----------|-------|----------|
| Init thành công | `POST /api/jobs/init` | `200` + `job_id` |
| Body thiếu field | `POST /api/jobs/init` với `{}` | `400` |

---

## 6. Admin — Quản trị

### 6.1 Quản lý users

| Test case | Input | Expected |
|-----------|-------|----------|
| Danh sách users | `GET /api/admin/users` (admin) | `200` + array users |
| Không phải admin | Gọi với user role `user` | `403` |
| Duyệt user | `POST /api/admin/users/{id}/approve` | `200` + `approved: true` |
| Duyệt user không tồn tại | `POST /api/admin/users/nonexistent/approve` | `404` |
| Duyệt user đã duyệt rồi | Gọi approve lần 2 | `200` (idempotent) |

### 6.2 Xem history user khác (admin)

| Test case | Input | Expected |
|-----------|-------|----------|
| Xem history user | `GET /api/admin/users/{id}/history` | `200` + history |
| User không tồn tại | `GET /api/admin/users/nonexistent/history` | `404` |

---

## 7. Utility — Tiện ích

### 7.1 Extract text từ file

| Test case | Input | Expected |
|-----------|-------|----------|
| Extract .txt | `POST /api/extract-text` với file .txt | `200` + `{text: ...}` |
| Extract .pdf | `POST /api/extract-text` với file .pdf | `200` + extracted text |
| Extract .docx | `POST /api/extract-text` với file .docx | `200` + extracted text |
| File không hỗ trợ | Upload file .exe | `400` + `"unsupported format"` |
| File quá lớn | File > 32MB | `413` |
| Quá nhiều concurrent | 5+ request cùng lúc | `429` |

---

## 8. System — Hệ thống

### 8.1 Health & Readiness

| Test case | Input | Expected |
|-----------|-------|----------|
| Health check | `GET /health` | `200` + `{status: "ok", service: "ai-backend-go"}` |
| Ready check (OK) | `GET /ready` khi DB + Redis + Engine OK | `200` + `{status: "ready"}` |
| Ready check (DB down) | `GET /ready` khi DB không kết nối | `503` |
| Ready check (Redis down) | `GET /ready` khi Redis không kết nối | `503` |
| Ready check (manifest chưa load) | `GET /ready` khi engine chưa gửi manifest | `503` |

### 8.2 Redis failover

| Test case | Expected behavior |
|-----------|------------------|
| Redis hoàn toàn down | Auth fail (user cache miss), rate limit ko hoạt động, task queue ko push được → 503 |
| Redis restart nhẹ (có persistence) | Key cũ survive → task state vẫn còn nếu chưa TTL expire |
| Redis restart ko persistence | Tất cả key mất → task/audio 404, rate limit counter reset về 0 |
| Redis memory full (eviction) | Key cũ bị evict (TTL 5 phút) → task/audio 404 |
| Redis network partition | API không rate-limit được, user cache miss → fallback xuống DB |
| Redis replication (master down) | Nếu có replica → read vẫn được, write fail |
| Redis AOF corruption | Redis ko start được → same as "Redis hoàn toàn down" |
| Redis maxmemory-policy=allkeys-lru | Task key bị evict ngay cả khi chưa hết TTL |
| Redis slow commands (KEYS *) | Block các request khác → timeout |
| Redis OOM (overcommit memory) | Redis process bị kill → mất tất cả |
| Redis password change | Backend ko reconnect được → Auth fail |
| Redis TLS fail | Backend ko handshake được → 503 |

### 8.3 PostgreSQL failover

| Test case | Expected behavior |
|-----------|------------------|
| PG restart | User cache (Redis) survive 5s → login vẫn hoạt động tạm thời |
| PG long downtime (> 5 phút) | Auth không register/login được, history không đọc được, user cache hết hạn → ko ai login được |
| PG connection pool full | Concurrent request bị timeout → `500` |
| PG slow query (index missing) | History page load rất chậm → timeout |
| PG deadlock (2 concurrent update) | Transaction bị rollback → 500 |
| PG unique constraint violation | Register username trùng → 409 (đã handle) |
| PG replication lag (read replica) | User vừa approve xong → query replica chưa thấy → denied |
| PG connection refused | Query fail → 500 |
| PG max_connections exceeded | Kết nối mới bị reject → 503 |
| PG transaction isolation (repeatable read) | 2 request cùng lúc → behavior? |
| PG vacuum/analyze chạy | Performance degrade tạm thời |
| PG data corruption | Query fail → 500, cần restore từ backup |

### 8.4 Engine failover

| Test case | Expected behavior |
|-----------|------------------|
| Engine crash | Task đang chạy stuck → timeout → failed |
| Engine restart | Engine gửi manifest mới → `/api/info` cập nhật |
| Engine gRPC disconnect | `/api/synthesize` trả về `502` |
| Engine gửi manifest sai format | Validate fail → manifest không cập nhật |
| Engine gửi manifest thiếu field | Validate fail → manifest không cập nhật |
| Engine gửi manifest với engine_id khác | Backend ghi đè? Hay từ chối? |
| Engine gửi manifest với supported_modes rỗng | `[]` → UI ko có mode nào |
| Engine gửi capabilities sai (ko parse được) | Fallback về default |
| Engine gửi audio_spec sai | Fallback về default |
| Engine gRPC stream reset | Task processing → fail → retry? |
| Engine trả về error code | `UNIMPLEMENTED`, `INTERNAL`, `UNAVAILABLE` → mỗi loại xử lý khác nhau? |
| Engine chậm (latency > 30s) | Task vẫn processing, user chờ lâu |
| Engine OOM | Process die → task failed |
| Engine GPU error (CUDA error) | Trả về error message → task failed |
| Engine gửi nhiều manifest cùng lúc | Race condition → manifest cuối cùng thắng? |
| Engine gửi manifest với tốc độ rất cao (flood) | Backend bị overwhelm? |
| Engine disconnect → reconnect → disconnect | Circuit breaker? |

### 8.5 Network & Connectivity

| Test case | Expected behavior |
|-----------|------------------|
| DNS resolution fail (engine host) | Backend ko connect được → 502 |
| TLS handshake fail backend-engine | Connection refused → 502 |
| Backend restart khi đang xử lý task | Task trong queue vẫn tồn tại (Redis persist) → worker xử lý tiếp |
| Backend graceful shutdown | Request đang xử lý hoàn thành trước khi shutdown |
| Backend force kill | Request đang xử lý bị dropped, task trong queue vẫn còn |
| Container restart (docker restart) | Tương tự backend restart |
| Network latency cao (> 1s) giữa backend-engine | API response chậm, timeout |
| Container resource limit (CPU throttle) | Backend chậm, timeout |
| Disk I/O saturation (log quá nhiều) | Backend chậm |

### 8.6 Task queue & Worker

| Test case | Expected behavior |
|-----------|------------------|
| Queue rỗng | Worker BRPOP block → chờ task mới |
| Queue nhiều task | Worker xử lý tuần tự (FIFO) |
| Task timeout | Task xử lý quá 5 phút → Redis TTL expire → `404` |
| Task failed | Worker ghi `status: error` + reason → API trả về lỗi |
| Queue message bị corrupt | Worker ko parse được → log error, skip message |
| Queue message bị duplicate | Worker process 2 lần → duplicate task? |
| Worker crash khi đang xử lý | Task stuck "processing" → ko ai cleanup |
| Worker restart | Worker nhận task mới, task cũ stuck "processing" |
| Brpoplpush (reliable queue) | Nếu có backup queue → task ko bị mất khi worker crash |
| Queue backlog (1000+ task) | Task cuối cùng chờ rất lâu → TTL expire |
| Queue bị flush (flushall) | Mất tất cả pending task |

### 8.7 Rate Limiting

| Test case | Expected |
|-----------|----------|
| Auth rate limit (register/login) | 10 request/phút → request 11+ bị `429` |
| Refresh rate limit | 10 request/phút riêng biệt với login |
| Extract rate limit | 5 request/phút (concurrency 4) |
| Synthesize rate limit | 20 request/phút? |
| Rate limit reset | Sau 1 phút, counter reset → request thành công |
| Rate limit window sliding | Request ở giây 59 và giây 61 → thuộc 2 window khác nhau |
| Rate limit bằng Redis key TTL | Redis down → rate limit ko hoạt động |
| Rate limit per user | User A gọi 10 lần → 429, User B gọi → 200 |
| Rate limit per IP | Same IP, nhiều user → shared rate limit? |
| Rate limit per endpoint | Login 10 lần + register 10 lần → riêng biệt, ko ảnh hưởng |
| Distributed rate limit (multi instance) | 2 backend instance → shared Redis counter → chính xác |
| Rate limit header trả về | `X-RateLimit-Remaining`, `X-RateLimit-Reset` → có đúng ko? |

### 8.8 Concurrency

| Test case | Expected |
|-----------|----------|
| Extract concurrency | 4+ request cùng lúc → request 5+ bị `429` |
| Synthesize concurrency | 20+ task cùng lúc → tùy config |
| Synthesize concurrency + stream | 20 stream cùng lúc → 21st bị 429 |
| Voice upload concurrency | 5+ upload cùng lúc → behavior? |
| Race condition: cancel + complete cùng lúc | Task vừa complete vừa cancel → trạng thái cuối là gì? |
| Race condition: audio download + task expire | Audio đang download → TTL expire → download bị cắt? |
| Race condition: 2 login cùng lúc | Cùng user, 2 device → cả 2 đều thành công? |
| Race condition: 2 register cùng lúc cùng username | 1 thành công (201), 1 fail (409) |

---

## 9. Edge Cases — Các trường hợp đặc biệt

| Test case | Expected |
|-----------|----------|
| Gửi JSON sai Content-Type | `Content-Type: text/plain` → server vẫn parse được (lỏng) | `201`/`200` (ko reject) |
| Body quá lớn | Payload > 10MB → `413` |
| Token sai chữ ký | Token tự ký sai → `401` |
| Token cũ nhưng chưa hết hạn | Token vẫn hợp lệ trong 15 phút → `200` |
| 2 request cùng lúc với cùng user | Rate limit tính riêng cho mỗi endpoint |
| Synthesize với text rỗng | `""` → `400` |
| Synthesize với special chars | Unicode, emoji, control chars → vẫn synthesize |
| Model_id không match | `zero_shot_clone` mà không gửi reference audio | Phải báo lỗi |
| Request có query params lạ | `?foo=bar` → ignored (không ảnh hưởng) |
| Range request (partial content) | `Range: bytes=0-1023` | `200` (server ko support range, trả full file) |

---

## 10. Test Priority Matrix

```
P0 — Critical (test ngay, fail = không release)
├── Register/Login/Refresh/Logout
├── Synthesize thành công + lấy audio (happy path)
├── Health + Ready
├── Manifest trả về đúng cấu trúc
├── Unapproved user không synthesize được
├── Engine gRPC disconnect → 502
└── Redis down → 503

P1 — High (test trước khi release)
├── Synthesize input validation (text quá dài, rỗng, model ko tồn tại)
├── Rate limit hoạt động (auth, synthesize, extract)
├── Task cancel (queued, processing, done, already cancelled)
├── Task state transitions (queued → processing → done)
├── Voice upload (thành công, file quá lớn, sai format)
├── History + pagination
├── Admin approve user
├── Extract text (.txt, .pdf, .docx)
├── Token hết hạn → 401
├── Streaming synthesis (normal, abort, chunk)
├── SSE task progress (done, error, cancelled)
├── Concurrency limit (synthesize, extract)
├── Audio download (success, expired, wrong user)
└── Cluster failover (Redis restart, PG restart, engine restart)

P2 — Medium (test nếu có thời gian)
├── Engine crash → task failed
├── Redis failover (memory full, partition, replication)
├── PG failover (connection pool, slow query, deadlock)
├── Concurrent extract (4+ → 429)
├── Queue FIFO + backlog
├── Queue message corrupt / duplicate
├── Worker crash mid-task
├── Swagger
├── Network partition backend ↔ engine
├── Rate limit window sliding
├── Race condition (cancel + complete, 2 register cùng lúc)
└── Streaming token hết hạn giữa chừng

P3 — Low (khi cần thiết)
├── Query params lạ
├── Special chars trong text
├── 2 request cùng lúc
├── Range request (partial content)
├── Disk I/O / CPU throttle
├── Container resource limit
└── Engine manifest flood
```

---

## 11. Cách chạy test

```bash
# Chuẩn bị
docker compose up -d

# Admin user
# username: admin, password: devadmin

# Test user (cần approve)
# username: testuser, password: testpass123

# Endpoint base
# http://localhost:5173/api
```

---

> Tổng cộng: ~200+ test cases, 8 nhóm luồng chính + edge cases.
> Trong đó luồng Synthesis chiếm ~100 test case (bao gồm tất cả hệ thống phụ: Redis, PG, Engine, Queue, Network, Race condition).
> Viết xong document này rồi, sang bước viết script test cụ thể nếu cần.