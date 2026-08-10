# 🛠️ Better TTS UI Studio - Technical Developer Guide

![Go 1.22](https://img.shields.io/badge/Control%20Plane-Go%201.22-00ADD8.svg)
![Svelte 5](https://img.shields.io/badge/Frontend-Svelte%205-orange.svg)
![PostgreSQL 16](https://img.shields.io/badge/Database-PostgreSQL%2016-336791.svg)
![Redis](https://img.shields.io/badge/Cache-Redis-DC382D.svg)
![Docker Compose](https://img.shields.io/badge/Deployment-Docker%20Compose-2496ED.svg)

> **Tài liệu Kỹ thuật dành cho Software Developers & Maintainers**  
> Hướng dẫn toàn diện về Kiến trúc Hệ thống (System Architecture), Quản lý Bộ nhớ Tải cao (High-Concurrency Memory Stability), Luồng Dữ liệu (Data Pipeline), Bảo mật (Security/Auth) và Hướng dẫn Phát triển (Development Workflow).

---

## 📋 Mục Lục
1. [Kiến trúc Hệ thống Decoupled (System Architecture)](#1-kiến-trúc-hệ-thống-decoupled-system-architecture)
2. [Tối ưu hóa Bộ nhớ & Tải cao (High-Concurrency Memory Architecture)](#2-tối-ưu-hóa-bộ-nhớ--tải-cao-high-concurrency-memory-architecture)
3. [Mô hình Bảo mật & Xác thực (Security & Auth Model)](#3-mô-hình-bảo-mật--xác-thực-security--auth-model)
4. [Quản lý Trạng thái & Pub/Sub Task Pipeline](#4-quản-lý-trạng-thái--pubsub-task-pipeline)
5. [Cấu trúc Mã nguồn & Code Directory Map](#5-cấu-trúc-mã-nguồn--code-directory-map)
6. [Cấu hình Hệ thống & Environment Reference](#6-cấu-hình-hệ-thống--environment-reference)
7. [Hướng dẫn Phát triển & Testing (Developer Workflow)](#7-hướng-dẫn-phát-triển--testing-developer-workflow)

---

## 1. 🏗️ Kiến trúc Hệ thống Decoupled (System Architecture)

Hệ thống được thiết kế theo mô hình **Decoupled 3-Tier Architecture** hoàn toàn tách biệt giữa Web Control Plane, Svelte 5 Frontend và AI Compute Engine:

```
                               ┌────────────────────────────────────────┐
                               │       Client Browser (Svelte 5)        │
                               │ - Svelte Runes & Audio Pipeline        │
                               │ - Dynamic UI Rendering via Schema      │
                               └──────────────────┬─────────────────────┘
                                                  │ REST API / SSE / Cookies
                                                  ▼
                               ┌────────────────────────────────────────┐
                               │      Control Plane Gateway (Go 1.22)   │
                               │ - Auth & Session Manager (JWT)         │
                               │ - Task Queue & Pub/Sub (Redis)         │
                               │ - Metadata & History (PostgreSQL/SQLc) │
                               │ - File Storage Engine (Local / S3)     │
                               └──────────────────┬─────────────────────┘
                                                  │ gRPC / REST Internal
                                                  ▼
                               ┌────────────────────────────────────────┐
                               │    Compute Engine Service (Python)     │
                               │ - PyTorch / ONNX GPU Audio Inference   │
                               │ - Dynamic Universal Manifest           │
                               └────────────────────────────────────────┘
```

### Thành phần chính:
- **Frontend (Svelte 5 Runes)**: Không chứa logic mã hóa cứng (hardcoded) của bất kỳ AI Model nào. Giao diện UI (Form controls, sliders, notification banners, rules) tự động dựng từ JSON Schema Manifest do Core Engine trả về tại `GET /api/info`.
- **Control Plane Gateway (Go Chi 1.22)**: Gateway chịu trách nhiệm routing dưới độ trễ miligiây, xác thực JWT, quản lý session, rate limiting, ghi nhận history vào PostgreSQL, và phân phối công việc qua Redis.
- **Compute Engine (Python FastAPI/PyTorch)**: Chạy ứng dụng AI Inference độc lập trên GPU. Cung cấp API nội bộ và Manifest schema.
- **Storage Layer**: Hỗ trợ lưu trữ file âm thanh linh hoạt qua đĩa cục bộ (Local Storage) hoặc MinIO/S3 Object Storage thông qua cờ `STORAGE_BACKEND`.

---

## 2. ⚡ Tối ưu hóa Bộ nhớ & Tải cao (High-Concurrency Memory Architecture)

Backend được tinh chỉnh để chịu tải **5,000+ Concurrent Virtual Users (VUs)** mà vẫn duy trì chân bộ nhớ RAM cực thấp (**~16 MiB Baseline**):

```
+-----------------------------------------------------------------------------------+
|                        Go Runtime Memory Stability Architecture                   |
|                                                                                   |
|  [ TASK_MEMORY_RETENTION_SECONDS=0 ] ---> Evicts finished tasks from RAM instantly |
|  [ ENABLE_REQUEST_LOGGING=false ]    ---> Bypasses sync.Pool buffer expansion     |
|  [ ENABLE_PPROF=false/true ]         ---> On-demand Heap Profiling (/debug/pprof) |
|  [ ENABLE_SWAGGER=true ]             ---> Serves OpenAPI UI (/swagger)            |
+-----------------------------------------------------------------------------------+
```

### Cơ chế Tối ưu hóa Đặc biệt:
1. **Chế độ Zero-Retention (`TASK_MEMORY_RETENTION_SECONDS=0`)**:
   - Mặc định các task hoàn thành được giữ trong RAM cache của `TaskManager` để tăng tốc độ truy vấn.
   - Khi đặt `= 0`, task sau khi hoàn thành sẽ **thu hồi khỏi RAM ngay lập tức**. Trạng thái task sẽ được lấy trực tiếp từ Redis/PostgreSQL khi Client polling, đảm bảo bộ nhớ không bị phình theo số lượng job.
2. **Kiểm soát Request Logging (`ENABLE_REQUEST_LOGGING=false`)**:
   - Dưới tải 5,000 VUs (~6,000 requests/giây), việc ghi log request bằng `chiMiddleware.Logger` sẽ tạo ra hàng vạn chuỗi log/giây, khiến `sync.Pool` của package `log` chuẩn phình to và kẹt RAM.
   - Tắt request logging trên Production giúp giảm đỉnh bộ nhớ từ **`192 MiB` xuống `16.65 MiB` (giảm 11.5 lần)**.
3. **Go Live Profiler (`ENABLE_PPROF=true`)**:
   - Tích hợp sẵn endpoint `/debug/pprof` để soi chi tiết từng Dòng Code / Struct đang chiếm dụng Heap Memory bằng lệnh:
     ```bash
     go tool pprof http://localhost:8000/debug/pprof/heap
     ```

---

## 3. 🔐 Mô hình Bảo mật & Xác thực (Security & Auth Model)

### Quy trình Xác thực (Authentication Pipeline):
1. **Dual-Token JWT Architecture**:
   - Access Token (TTL ngắn) và Refresh Token (TTL dài) được mã hóa HMAC-SHA256 với `SECRET_KEY`.
   - **Xác thực kép**: Hệ thống ưu tiên đọc từ **HttpOnly Cookie** (`access_token`) để chống tấn công XSS trên Web Frontend. Nếu không tìm thấy, hệ thống sẽ fallback kiểm tra header `Authorization: Bearer <token>` phục vụ API Client / Postman.
2. **User Record Caching (`AUTH_USER_CACHE_SECONDS`)**:
   - Để tránh việc mỗi request gửi lên đều phải thực hiện truy vấn `SELECT` xuống PostgreSQL, middleware giữ một bộ đệm ngắn hạn (`userCache`).
   - Khi Admin phê duyệt tài khoản hoặc thay đổi quyền, hàm `InvalidateUser(username)` sẽ được kích hoạt để xoá cache tức thì trên cả RAM và Redis.
3. **Rate Limiting Middleware**:
   - Middleware `RateLimit` dựa trên Thuật toán Cửa sổ trượt (Sliding Window) hỗ trợ bởi Redis.
   - Hạn chế tấn công dò mật khẩu tại `/api/login` và `/api/register`.
4. **Body Limit Protection**:
   - Áp dụng `middleware.BodyLimit` để chặn các request chứa Payload lớn bất thường ngay từ tầng Router, trả về lỗi HTTP `413 Payload Too Large`.

---

## 4. 🗄️ Quản lý Trạng thái & Pub/Sub Task Pipeline

### Vòng đời Task (Task Lifecycle):
```
  [ POST /api/synthesize/{mode} ]
                 │
                 ▼
       (Status: "pending") ─── Ghi PostgreSQL & Redis Queue
                 │
                 ▼
       (Status: "processing") ── Worker nhận Job & gọi Engine gRPC/REST
                 │
                 ├──────► [ SSE Stream ] Push tiến độ % qua Redis Pub/Sub
                 │
                 ▼
       (Status: "completed" / "failed")
                 │
                 ▼
 [ Cleanup via Retention Logic ] ── Evict RAM nếu TASK_MEMORY_RETENTION_SECONDS=0
```

- **Database Metadata**: PostgreSQL quản lý bảng `users`, `tasks`, `voices` thông qua SQL thuần được biên dịch thành Go Code an toàn bằng **SQLc**.
- **Real-time Event Streaming**: Khi worker xử lý từng chunk âm thanh, tiến độ (%) được đẩy vào kênh **Redis Pub/Sub**. HTTP Server lắng nghe kênh này và truyền trực tiếp về Browser qua **Server-Sent Events (SSE)** tại `/api/tasks/{id}/events`.

---

## 5. 📂 Cấu trúc Mã nguồn & Code Directory Map

```text
better-tts-ui-studio/
├── backend/                  # Control Plane Web Backend (Go 1.22)
│   ├── cmd/
│   │   ├── gen-env/          # Công cụ sinh tự động .env.example từ settings.go
│   │   ├── web/              # Entry point chạy Web Backend Server
│   │   └── worker/           # Entry point chạy Background Queue Worker
│   ├── config/               # Hệ thống khai báo & kiểm tra cấu hình tập trung (settings.go, config.go)
│   ├── db/                   # Database access layer (SQLc generated structs & migrations)
│   ├── handlers/             # HTTP Handlers (Auth, Tasks, Voices, History, Swagger, Health)
│   ├── middleware/           # Middlewares (Auth, RateLimit, BodyLimit, UserCache)
│   ├── router/               # Khai báo Go-Chi Router & mount endpoints
│   ├── state/                # TaskManager, Redis connection & Task Pub/Sub state
│   ├── storage/              # Engine lưu trữ file (Local Disk / S3 MinIO)
│   └── tests/                # Integration tests & Unit tests
├── frontend/                 # Web Studio App (Svelte 5 Runes + Vite)
│   ├── src/
│   │   ├── lib/              # Components (DynamicForm, AudioPlayer, Header, Modal)
│   │   └── stores/           # State stores (Auth, Task progress, Engine manifest)
├── test/                     # k6 stress test scripts & Python RAM monitor tools
├── docker-compose.yml        # Orchestration file cho toàn bộ hệ thống
└── .env.example              # Tệp mẫu cấu hình môi trường được sinh tự động
```

---

## 6. ⚙️ Cấu hình Hệ thống & Environment Reference

Mọi biến môi trường của Backend đều được định nghĩa tập trung tại `backend/config/settings.go`. Các biến quan trọng:

| Biến môi trường | Loại | Mặc định | Mô tả kỹ thuật |
| :--- | :--- | :--- | :--- |
| `PORT` | String | `8000` | Cổng HTTP Server của Backend. |
| `SECRET_KEY` | Secret | *Bắt buộc* | Secret key dùng để ký và xác thực JWT Tokens. |
| `DATABASE_URL` | String | *PostgreSQL URL* | Connection string tới PostgreSQL. |
| `REDIS_URL` | String | `localhost:6379` | Địa chỉ kết nối Redis Server. |
| `TASK_MEMORY_RETENTION_SECONDS` | Int | `600` | Số giây giữ task trong RAM. Đặt `0` để **tắt hoàn toàn RAM Cache**. |
| `ENABLE_REQUEST_LOGGING` | Bool | `true` | Bật/Tắt log request ra stdout. Đặt `false` dưới tải cao để giảm tốn RAM. |
| `ENABLE_PPROF` | Bool | `false` | Bật/Tắt Go Profiler tại `/debug/pprof`. |
| `ENABLE_SWAGGER` | Bool | `true` | Bật/Tắt Swagger UI tại `/swagger`. |
| `STORAGE_BACKEND` | String | `local` | Kho lưu trữ âm thanh: `local` (Đĩa) hoặc `s3` (MinIO/S3). |
| `DB_MAX_CONNS` | Int | `25` | Kích thước tối đa của PostgreSQL Connection Pool. |

> 💡 **Mẹo**: Khi thêm một biến cấu hình mới vào `settings.go`, hãy chạy lệnh `go generate ./config` để cập nhật lại tệp `.env.example`.

---

## 7. 🛠️ Hướng dẫn Phát triển & Testing (Developer Workflow)

### 1. Khởi động môi trường Local với Docker Compose:
```bash
# Tạo file .env từ mẫu
cp .env.example .env

# Điền SECRET_KEY, POSTGRES_PASSWORD, REDIS_PASSWORD trong .env

# Dựng và chạy toàn bộ dịch vụ
docker compose up -d --build
```

### 2. Kiểm thử Unit Test Backend:
```bash
cd backend
go test ./...
```

### 3. Sinh tự động `.env.example` từ `settings.go`:
```bash
cd backend
go generate ./config
```

### 4. Chạy Stress Test tải cao (5,000 VUs) với k6:
```bash
# Kiểm tra khả năng hồi phục RAM trong 3.5 phút
docker run --rm -i --network=host -v $(pwd)/test:/test grafana/k6 run /test/test_quick_compare.js
```

### 5. Truy cập Endpoints Tài liệu & Debug:
- 📖 **Swagger UI**: `http://localhost:8000/swagger`
- 📄 **OpenAPI Spec**: `http://localhost:8000/swagger/doc.json`
- 🔍 **Go Heap Profiler** (Khi `ENABLE_PPROF=true`): `http://localhost:8000/debug/pprof/`

---

© 2026 Better TTS UI Studio Engineering Team. Distributed under the MIT License.
