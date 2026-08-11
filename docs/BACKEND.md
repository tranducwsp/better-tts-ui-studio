# 🔧 Đặc Tả Kỹ Thuật Backend (Backend Technical Specification)

Tài liệu này đặc tả chi tiết kiến trúc kỹ thuật của hệ thống **Control Plane Gateway** (được viết bằng ngôn ngữ **Go - Golang 1.22**), bao gồm các tầng kiến trúc, cơ chế xử lý Concurrency, bảo mật JWT, kết nối Database, Task Queue và tích hợp bộ lưu trữ.

---

## 🛠️ 1. Công Nghệ & Thư Viện Sử Dụng (Tech Stack)

| Thành Phần | Công Nghệ / Thư Viện | Mô Tả & Tối Ưu |
| :--- | :--- | :--- |
| **Language** | **Go 1.22** | Ngôn ngữ biên dịch hiệu năng cao, quản lý bộ nhớ RAM tối ưu và Goroutines concurrency vượt trội |
| **HTTP Router** | **Chi Router (`go-chi/chi/v5`)** | Lightweight, idiomatic Go HTTP router, tốc độ routing nhanh, hỗ trợ Middleware chaining linh hoạt |
| **JSON Parser** | **Sonic (`bytedance/sonic`)** | Thư viện JIT-compiled JSON serializer/deserializer tốc độ cao nhất trong hệ sinh thái Go |
| **Database Pool** | **`pgx/v5/pgxpool`** | Driver PostgreSQL giao tiếp trực tiếp qua binary protocol, quản lý pool kết nối concurrency cao |
| **Task Queue** | **Redis Streams (`go-redis/v9`)** | Hàng đợi công việc tin cậy (At-least-once processing, Consumer Groups), tự động fallback về Go Channel RAM nếu không có Redis |
| **Storage SDK** | **`aws-sdk-go-v2`** | SDK chính thức giao tiếp với AWS S3, MinIO, Cloudflare R2 |
| **Auth Security** | **JWT (`golang-jwt/jwt/v5`) + Bcrypt** | Mã hóa mật khẩu an toàn và ký Access/Refresh Tokens lưu trong Cookie HttpOnly |
| **Documentation** | **HttpSwagger (`swaggo/http-swagger`)** | Phục vụ tài liệu Swagger API động |

---

## 🏗️ 2. Các Tầng Mã Nguồn (Layered Architecture)

Mã nguồn Backend được chia thành các tầng module độc lập, áp dụng nguyên lý **Clean Architecture**:

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

## 🔒 3. Bảo Mật & Xác Thực (Authentication & Security)

### 3.1. Cơ Chế JWT Session Với Cookie HttpOnly

Hệ thống triển khai cơ chế xác thực kép an toàn:

- **Access Token**: Thời hạn ngắn (Mặc định `15 phút`), được lưu trong Cookie HttpOnly tên `access_token`. Dùng để xác thực tất cả các request API.
- **Refresh Token**: Thời hạn dài (Mặc định `30 ngày`), được lưu trong Cookie HttpOnly tên `refresh_token` với đường dẫn riêng `/api/auth/refresh`.
- **HttpOnly & Secure Flags**: Cookie được thiết lập cờ `HttpOnly` (chống bị đánh cắp qua tấn công XSS) và `SameSite=Lax`. Cờ `Secure` tự động được bật trên giao thức HTTPS (`COOKIE_SECURE=1`).

### 3.2. Mã Hóa Mật Khẩu & Bảo Vệ Tài Khoản

- Mật khẩu người dùng được băm (hash) bằng giải thuật **Bcrypt** trước khi lưu vào PostgreSQL.
- **Rate Limiting**: Các endpoint nhạy cảm như `/api/login` và `/api/register` được bảo vệ bởi Middleware Rate Limit (`AUTH_RATE_LIMIT_REQUESTS`, mặc định 10 lượt/phút) để ngăn chặn tấn công dò mật khẩu (Brute-force attack).
- **Phân Quyền Vai Trò (RBAC)**: Hệ thống chia làm 2 quyền chính: `user` và `admin`. Các tài khoản mới đăng ký phải qua trạng thái `pending` và được Admin chấp thuận qua `/api/admin/users/{user_id}/approve` trước khi có thể sử dụng tính năng tổng hợp.

---

## ⚡ 4. Điều Phối Hàng Đợi & Quản Lý Concurrency (Task Pipeline)

### 4.1. Khởi Tạo Job & Phân Đoạn (Chunking)

Khi client gửi yêu cầu tổng hợp:
1. `handlers/history.go`: Tiếp nhận request `POST /api/jobs/init`, ghi nhận bản ghi Job mới vào bảng `jobs` trong PostgreSQL.
2. `synth/pipeline.go`: Phân tách đoạn văn bản dài thành các phân đoạn (Chunk) nhỏ dựa trên giới hạn `max_chars` của Manifest.
3. Tạo các bản ghi Task Chunk tương ứng trong bảng `tasks` với trạng thái `pending`.

### 4.2. Đẩy Hàng Đợi & Xử Lý Đồng Thời

1. `queue/job_queue.go`: Đẩy các Task ID vào Redis Stream `tts:tasks`.
2. Các Worker Goroutines chạy ngầm trong `synth/pipeline.go` lắng nghe Stream qua Redis Consumer Group:
   - Giới hạn số Task xử lý đồng thời bởi `WORKER_MAX_IN_FLIGHT`.
   - Lấy Task -> Chuyển trạng thái `processing` -> Gọi AI Core TTS Engine.
   - Nhận Stream âm thanh đầu ra -> Đưa qua `ffmpeg` chuyển mã (nếu cần) -> Lưu file vào Storage -> Đánh dấu status `completed`.

---

## 💾 5. Tầng Lưu Trữ File (Storage Layer Abstraction)

Mã nguồn định nghĩa một Interface duy nhất trong `storage/store.go`:

```go
type Store interface {
    Save(ctx context.Context, key string, r io.Reader) (string, error)
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
}
```

Hệ thống cung cấp 2 bản triển khai:
1. **`LocalStorage` (`storage/local.go`)**: Ghi/Đọc file trực tiếp trên ổ đĩa cục bộ của máy chủ (`STORAGE_DIR`). Thích hợp cho chạy thử hoặc triển khai đơn nút.
2. **`S3Storage` (`storage/s3.go`)**: Ghi/Đọc file trên các hệ thống Cloud Object Storage (AWS S3, MinIO, Cloudflare R2). Thích hợp cho mô hình Scale nhiều Pods/Replicas trên Kubernetes.

---

## 📊 6. Tích Hợp Quản Lý Bộ Nhớ RAM & Performance Tuning

Để duy trì độ ổn định trên môi trường Production với hàng nghìn request đồng thời:

1. **User Cache In-Memory**: Lưu cache bản ghi User trong bộ nhớ tạm (`AUTH_USER_CACHE_SECONDS`) để giảm 90% số lượng truy vấn `SELECT` lặp lại tới PostgreSQL.
2. **Body Limit Middleware (`middleware/body_limit.go`)**: Khống chế kích thước tối đa của body HTTP request ngay tại tầng Router, từ chối sớm request có `Content-Length` quá lớn (tránh cạn kiệt RAM).
3. **Concurrency Limiter (`middleware/concurrency.go`)**: Đặt trần số lượng kết nối nặng đồng thời (Ví dụ: `/api/extract-text` khống chế tối đa `8` request đồng thời vì mỗi request trích xuất file PDF/DOCX tiêu tốn ~32MB RAM).
4. **Stale Task Sweeper (`database/sweeper.go`)**: Chạy ngầm định kỳ thu hồi và báo lỗi các Task bị kẹt quá lâu do Worker chết đột ngột hoặc rớt mạng.
