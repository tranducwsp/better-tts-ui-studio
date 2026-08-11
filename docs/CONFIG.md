# ⚙️ Hướng Dẫn Cấu Hình Biến Môi Trường (Environment Variables Guide)

Tài liệu này giải thích chi tiết toàn bộ các biến môi trường (Environment Variables) sử dụng cho Backend Gateway và hệ thống **Better TTS UI Studio**.

---

## 📌 Nguyên Lý Quản Lý Cấu Hình (Single Source of Truth)

Toàn bộ các biến môi trường của hệ thống Backend được định nghĩa tập trung tại một nơi duy nhất trong mã nguồn: file **`backend/config/settings.go`**.

> [!NOTE]
> File mẫu `.env.example` được sinh tự động hoàn toàn từ bảng khai báo trong `settings.go`. Nhờ đó, thông tin tài liệu, giá trị mặc định và kiểu dữ liệu luôn đồng bộ 100% với mã nguồn Go Backend.
> 
> Nếu bạn thêm hoặc chỉnh sửa biến cấu hình mới, hãy sửa tại `settings.go` và chạy lệnh sau để cập nhật `.env.example`:
> ```bash
> cd backend
> go generate ./config
> ```

---

## 📋 Danh Sách Chi Tiết Các Biến Môi Trường (ENV Reference)

### 1. Nhóm Bắt Buộc Trong Production (Required)

| Biến Môi Trường | Kiểu | Mặc Định | Mô Tả & Hướng Dẫn |
| :--- | :--- | :--- | :--- |
| `SECRET_KEY` | Secret | (Trống) | Khóa bí mật dùng để ký và xác thực JWT Session Token. **BẮT BUỘC**. Sinh khóa bằng lệnh: `openssl rand -hex 32` |
| `POSTGRES_PASSWORD` | Secret | (Trống) | Mật khẩu tài khoản cơ sở dữ liệu PostgreSQL. **BẮT BUỘC**. Sinh mật khẩu bằng: `openssl rand -hex 24` |
| `REDIS_PASSWORD` | Secret | (Trống) | Mật khẩu truy cập Redis Server. Nên đặt cho môi trường Production. |

---

### 2. Nhóm Khởi Tạo Tài Khoản Mẫu (Seed Accounts)

| Biến Môi Trường | Kiểu | Mặc Định | Mô Tả |
| :--- | :--- | :--- | :--- |
| `DEFAULT_ADMIN_USERNAME` | String | (Trống) | Tên tài khoản Admin sẽ được tự động khởi tạo khi chạy ứng dụng lần đầu |
| `DEFAULT_ADMIN_PASSWORD` | Secret | (Trống) | Mật khẩu của tài khoản Admin khởi tạo |
| `DEFAULT_USER_USERNAME` | String | (Trống) | Tên tài khoản User mẫu khởi tạo |
| `DEFAULT_USER_PASSWORD` | Secret | (Trống) | Mật khẩu của tài khoản User mẫu |

---

### 3. Nhóm Cấu Hình Server & HTTP Gateway

| Biến Môi Trường | Kiểu | Mặc Định | Khoảng Tối Đa/Tối Thiểu | Mô Tả |
| :--- | :--- | :--- | :--- | :--- |
| `HOST` | String | `0.0.0.0` | - | Địa chỉ IP interface lắng nghe kết nối của HTTP Gateway |
| `PORT` | String | `8000` | - | Cổng mạng của Backend HTTP Gateway |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | Int | `15` | 1 - 525600 | Thời hạn hết hiệu lực của JWT Access Token (phút) |
| `REFRESH_TOKEN_EXPIRE_MINUTES` | Int | `43200` | 60 - 525600 | Thời hạn hết hiệu lực của Refresh Token Cookie (30 ngày) |
| `AUTH_USER_CACHE_SECONDS` | Int | `15` | 0 - 3600 | Thời gian cache thông tin User trong RAM để giảm tải truy vấn DB. Đặt `0` để tắt cache |
| `TASK_MEMORY_RETENTION_SECONDS`| Int | `600` | 0 - 86400 | Thời gian giữ trạng thái Task đã xong trong RAM cache. Đặt `0` để giải phóng RAM ngay |
| `ENABLE_REQUEST_LOGGING` | Bool | `true` | true / false | Bật/Tắt ghi log chi tiết từng request HTTP ra stdout (Tắt dưới tải cao để tối ưu RAM) |
| `ENABLE_PPROF` | Bool | `false` | true / false | Bật/Tắt Go Profiler endpoint (`/debug/pprof`) soi bộ nhớ Heap/Goroutines |
| `ENABLE_SWAGGER` | Bool | `true` | true / false | Bật/Tắt trang tài liệu OpenAPI / Swagger UI (`/swagger`) |

---

### 4. Nhóm Kết Nối Cơ Sở Dữ Liệu PostgreSQL

| Biến Môi Trường | Kiểu | Mặc Định | Mô Tả |
| :--- | :--- | :--- | :--- |
| `DATABASE_URL` | String | `postgres://postgres:...@localhost:5432/ai_studio` | Chuỗi kết nối PostgreSQL (Connection String). Khi chạy Docker Compose, biến này tự được lắp ghép |
| `DB_MAX_CONNS` | Int | `25` | Số lượng kết nối tối đa trong Connection Pool (`pgxpool`) |
| `DB_MIN_CONNS` | Int | `5` | Số lượng kết nối duy trì tối thiểu trong Pool (không được lớn hơn `DB_MAX_CONNS`) |
| `DB_MAX_CONN_LIFETIME_MINUTES` | Int | `30` | Thời gian sống tối đa của một kết nối DB trước khi tạo lại |
| `DB_MAX_CONN_IDLE_MINUTES` | Int | `15` | Thời gian tối đa kết nối rảnh rỗi nằm trong pool |
| `DB_CONNECT_MAX_RETRIES` | Int | `10` | Số lần thử kết nối lại DB khi hệ thống vừa khởi động |
| `DB_CONNECT_RETRY_INTERVAL_SECONDS` | Int | `2` | Khoảng thời gian chờ giữa mỗi lần kết nối lại DB |

---

### 5. Nhóm Cấu Hình Hàng Đợi & Redis Cache (Redis Optional)

| Biến Môi Trường | Kiểu | Mặc Định | Mô Tả |
| :--- | :--- | :--- | :--- |
| `REDIS_URL` | String | `localhost:6379` | Địa chỉ kết nối Redis Server. Nếu bỏ trống, Gateway sẽ chạy ở chế độ In-Memory Queue |
| `REDIS_PASSWORD` | Secret | (Trống) | Mật khẩu xác thực với Redis Server |

---

### 6. Nhóm Kết Nối AI Core Engine & Frontend Builder

| Biến Môi Trường | Kiểu | Mặc Định | Mô Tả |
| :--- | :--- | :--- | :--- |
| `CORE_ENGINE_URL` | String | `http://localhost:8001` | URL HTTP của dịch vụ Python AI Core TTS Engine (nơi lấy Manifest & Synthesize) |
| `CORE_ENGINE_GRPC_URL` | String | `localhost:50051` | Địa chỉ gRPC Server của AI Core TTS Engine (nếu dùng gRPC) |
| `TTS_CLIENT_TIMEOUT_SECONDS` | Int | `60` | Thời gian timeout tối đa cho một yêu cầu gọi tổng hợp tới AI Core Engine |
| `FE_BUILDER_URL` | String | `http://frontend-builder:3001` | URL của dịch vụ Node.js Builder nhận Webhook reload UI khi Manifest thay đổi |
| `VITE_BACKEND_URL` | String | `http://backend:8000` | Địa chỉ Backend Gateway mà Frontend Builder & Dev Server sử dụng để đọc Manifest |

---

### 7. Nhóm Lưu Trữ File & Hạn Mức (Storage & Limits)

| Biến Môi Trường | Kiểu | Mặc Định | Mô Tả |
| :--- | :--- | :--- | :--- |
| `STORAGE_BACKEND` | String | `local` | Chọn kho lưu trữ file: `"local"` (Ổ đĩa local) hoặc `"s3"` (AWS S3 / MinIO / Cloudflare R2) |
| `STORAGE_DIR` | String | `storage` | Thư mục lưu trữ chính khi `STORAGE_BACKEND=local` |
| `S3_BUCKET` | String | (Trống) | Tên S3 Bucket (Bắt buộc khi `STORAGE_BACKEND=s3`) |
| `S3_REGION` | String | `us-east-1` | Vùng (Region) của S3 Bucket |
| `S3_ENDPOINT` | String | (Trống) | Endpoint S3 tùy chỉnh (Ví dụ: `https://s3.example.com` cho MinIO/R2) |
| `S3_ACCESS_KEY_ID` | Secret | (Trống) | Access Key ID cho S3 |
| `S3_SECRET_ACCESS_KEY` | Secret | (Trống) | Secret Access Key cho S3 |
| `S3_FORCE_PATH_STYLE` | Int | `0` | Đặt `1` khi dùng MinIO (truy cập đường dẫn dạng `endpoint/bucket/key`) |
| `S3_PREFIX` | String | (Trống) | Tiền tố thư mục trên S3 để phân biệt môi trường (`prod`, `staging`) |
| `MAX_UPLOAD_SIZE_MB` | Int | `50` | Trần cứng dung lượng upload file tối đa (MB) của hạ tầng Gateway |
| `TEMP_AUDIO_RETENTION_HOURS` | Int | `24` | Số giờ giữ lại các file audio tạm trong `storage/temp/` trước khi Sweeper xóa |
| `PRESERVE_FILES` | Int | `0` | Đặt `1` để giữ lại file âm thanh mẫu trên đĩa khi xóa giọng Clone trong DB |

---

### 8. Nhóm Bảo Mật Mạng & Tối Ưu Nâng Cao

| Biến Môi Trường | Kiểu | Mặc Định | Mô Tả |
| :--- | :--- | :--- | :--- |
| `COOKIE_SECURE` | Int | `1` | Đặt `1` để bật cờ `Secure` cho Cookie (chỉ gửi qua HTTPS). Đặt `0` khi dev ở `http://localhost` |
| `CORS_ALLOWED_ORIGINS` | List | `http://localhost:5173,...` | Danh sách tên miền được phép truy cập API (phân tách bằng dấu phẩy) |
| `TRUSTED_PROXIES` | List | (Trống) | Dải CIDR Proxy tin cậy (`10.0.0.0/8`) được phép đặt header `X-Forwarded-For` |
| `WORKER_MAX_IN_FLIGHT` | Int | `2` | Số lượt tổng hợp chạy đồng thời tối đa trong mỗi Worker tiến trình |
| `TRANSCODE_MAX_CONCURRENCY` | Int | `2` | Số tiến trình `ffmpeg` chuyển mã âm thanh chạy đồng thời tối đa |
| `STALE_CHUNK_AFTER_MINUTES` | Int | `30` | Số phút tối đa cho một Task Chunk bị treo trước khi Sweeper đánh dấu lỗi `failed` |
| `AUTH_RATE_LIMIT_REQUESTS` | Int | `10` | Số lượt tối đa được phép gọi API Đăng nhập/Đăng ký trong một cửa sổ |
| `AUTH_RATE_LIMIT_WINDOW_SECONDS` | Int | `60` | Độ dài cửa sổ thời gian Rate Limit cho Login/Register (giây) |
