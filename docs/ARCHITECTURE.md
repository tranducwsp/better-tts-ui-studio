# 🏗️ Kiến Trúc Hệ Thống & Luồng Dữ Liệu (Deployed Architecture & Data Flow)

Tài liệu này đặc tả cấu trúc tổng thể của hệ thống **Better TTS UI Studio** khi triển khai trên môi trường Production, cách các thành phần giao tiếp với nhau, luồng dữ liệu (Data Flow), cơ chế hoạt động của các Task Worker và tiến trình ngầm Cron/Sweeper.

---

## 🏛️ 1. Sơ Đồ Kiến Trúc Hệ Thống Triển Khai (Deployed Architecture)

Khi được triển khai hoàn chỉnh (thông qua Docker Compose hoặc Kubernetes), hệ thống phân tách làm 3 tầng độc lập: **Presentation Layer (Frontend)**, **Control Plane Gateway (Backend Go)** và **Compute Layer (Python AI Engine)**.

```mermaid
flowchart TB
    subgraph Client [" Trình Duyệt Người Dùng (Browser) "]
        UI["Svelte 5 App (Dynamic UI)"]
        AudioEngine["Web Audio API Engine (PCM Player)"]
    end

    subgraph FrontendServer [" Frontend Builder & Web Server "]
        Nginx["Nginx Static Web Server (Port 5173)"]
        Builder["Node.js Prerender Builder (Port 3001)"]
    end

    subgraph ControlPlane [" Control Plane Gateway (Go Chi - Port 8000) "]
        Router["Chi HTTP Router & Auth Middleware"]
        JobManager["Job & Task Pipeline Manager"]
        RedisQueue["Redis Stream Queue Producer/Consumer"]
        Transcoder["ffmpeg Transcode Pool"]
        StorageEngine["Storage Abstraction Layer"]
    end

    subgraph Infrastructure [" Cơ Sở Dữ Liệu & Bộ Nhớ "]
        PG[("PostgreSQL 16 (Users, Jobs, Voices)")]
        RedisDB[("Redis (Stream Queue & Auth Cache)")]
        DiskStorage[("Storage System (Local Disk / AWS S3)")]
    end

    subgraph ComputeEngine [" Compute Engine (Python Microservice - Port 8001/50051) "]
        FastAPI["FastAPI App (GET /info, GET /voices)"]
        PyTorch["PyTorch / CUDA Inference Engine"]
        gRPCServer["gRPC Synthesize Server (Optional)"]
    end

    UI <-->|"REST API & SSE Stream (Cookie Session)"| Router
    UI -->|"Tải HTML/JS/CSS Static Bundle"| Nginx
    Router <-->|"Ghi & Truy vấn dữ liệu"| PG
    Router <-->|"Task Streams & User Cache"| RedisDB
    JobManager <-->|"Lưu trữ file âm thanh & giọng Clone"| StorageEngine
    StorageEngine <-->|"Ghi/Đọc file"| DiskStorage
    
    JobManager <-->|"HTTP / gRPC Synthesis Request"| FastAPI
    JobManager <-->|"gRPC Audio Stream"| gRPCServer

    Router -->|"Kích hoạt Reload Webhook"| Builder
    Builder -->|"Đọc /api/info lấy Manifest"| Router
    Builder -->|"Dựng lại index.html Bundle"| Nginx
```

---

## 🔄 2. Chi Tiết Luồng Dữ Liệu (Data Flow)

### 2.1. Luồng Đồng Bộ Manifest (`GET /info` & Webhook Reload)

Khi AI Engineer khởi chạy hoặc cập nhật mô hình AI:

```mermaid
sequenceDiagram
    autonumber
    participant Admin as Admin / CI Pipeline
    participant GoGateway as Go Control Plane Gateway
    participant PythonEngine as Python Core TTS Engine
    participant Builder as Frontend Builder (Node.js)
    participant Client as Frontend Browser (Svelte 5)

    GoGateway->>PythonEngine: Khởi động: GET /info (Lấy Engine Manifest)
    PythonEngine-->>GoGateway: Trả về JSON Manifest (Model, UI Schema, Regex Rules)
    GoGateway->>GoGateway: Validate & Cache Manifest trong bộ nhớ RAM
    
    Admin->>GoGateway: Gọi POST /api/internal/engine/reload
    GoGateway->>PythonEngine: Gửi lại GET /info lấy Manifest mới
    GoGateway->>Builder: Gọi HTTP Webhook (POST FE_BUILDER_URL/reload)
    Builder->>GoGateway: Gọi GET /api/info để lấy Manifest mới nhất
    Builder->>Builder: Thực thi script prerender dựng lại index.html
    GoGateway-->>Client: Người dùng truy cập/reload nhận ngay UI mới
```

---

### 2.2. Luồng Tổng Hợp Tiếng Nói (Text-to-Speech Processing Flow)

Đây là luồng chính khi người dùng tạo âm thanh từ văn bản:

```mermaid
sequenceDiagram
    autonumber
    participant Client as Frontend Svelte 5
    participant Gateway as Go Gateway Router
    participant DB as PostgreSQL
    participant Redis as Redis Stream Queue
    participant Worker as Go Task Worker Pipeline
    participant Python as Python AI Core Engine
    participant Storage as Storage (Disk / S3)

    Client->>Client: Áp dụng Regex Rules tự chuẩn hóa văn bản tại Client
    Client->>Client: Chia đoạn văn bản thành các Chunk nhỏ (theo giới hạn max_chars)
    Client->>Gateway: POST /api/jobs/init (Khởi tạo Job tổng hợp mới)
    Gateway->>DB: Tạo bản ghi Job và danh sách các Chunks trạng thái 'pending'
    Gateway-->>Client: Trả về Job ID và Danh sách Task IDs

    Client->>Gateway: POST /api/synthesize/{model_id} (Gửi từng Task Chunk)
    Gateway->>Redis: Xử lý đẩy Task vào Redis Stream Queue `tts:tasks`
    Gateway-->>Client: Trả về HTTP 202 Accepted (Task ID)

    Client->>Gateway: Kết nối SSE /api/stream/tasks/{task_id} lắng nghe tiến độ

    loop Tiến Trình Xử Lý Trong Queue
        Worker->>Redis: XreadGroup lấy Task cần xử lý từ Queue
        Worker->>DB: Cập nhật trạng thái Task thành 'processing'
        Worker->>Python: Gửi HTTP POST /tts (hoặc gRPC Synthesize) kèm tham số
        Python->>Python: Chạy PyTorch/CUDA Model sinh ra Audio Binary (WAV)
        Python-->>Worker: Trả về Audio Stream Binary
        Worker->>Worker: Chuyển đổi định dạng audio bằng ffmpeg (nếu cần)
        Worker->>Storage: Ghi file âm thanh vào kho lưu trữ (storage/audio/{id}.wav)
        Worker->>DB: Cập nhật Task status='completed', lưu file_path
        Worker->>Gateway: Bắn sự kiện SSE Event: `progress` & `completed`
    end

    Gateway-->>Client: Sự kiện SSE thông báo Task đã hoàn tất
    Client->>Gateway: GET /api/tasks/{task_id}/audio (Tải audio binary)
    Gateway->>Storage: Đọc file âm thanh
    Gateway-->>Client: Trả về Audio File Buffer (Audio/WAV)
    Client->>Client: Web Audio API phát đoạn âm thanh nối tiếp trên giao diện
```

---

### 2.3. Luồng Tải Lên Giọng Mẫu Clone (Voice Cloning Flow)

```mermaid
sequenceDiagram
    autonumber
    participant Client as Frontend WaveformTrimmer
    participant Gateway as Go Gateway
    participant Storage as Storage (Disk / S3)
    participant Python as Python AI Core Engine
    participant DB as PostgreSQL

    Client->>Client: Cắt và chọn phân đoạn âm thanh chất lượng tốt nhất
    Client->>Gateway: POST /api/clone/upload (File WAV + Metadata: Name, Gender, Accent, Age)
    Gateway->>Gateway: Kiểm tra dung lượng file (không vượt quá max_upload_bytes của Manifest)
    Gateway->>Storage: Lưu trữ file tham chiếu vào `storage/{user_id}/voice/{id}.wav`
    
    alt Nếu Core Engine cần đăng ký trước (Register Endpoint)
        Gateway->>Python: Gọi POST /clone gửi file âm thanh tham chiếu
        Python-->>Gateway: Trả về embedding_id hoặc voice_id từ Model
    end

    Gateway->>DB: Ghi nhận thông tin Voice Clone mới gắn với user_id
    Gateway-->>Client: Trả về thông tin Voice Clone vừa khởi tạo
    Client->>Client: Cập nhật danh sách giọng đọc trong VoiceSelect Dropdown
```

---

## ⚙️ 3. Quản Lý Tiến Trình Worker (Task Workers & Concurrency)

Hệ thống xử lý hàng đợi tổng hợp tiếng nói thông qua hai chế độ hoạt động:

1. **Chế Độ Distributed Queue (Khi có Redis)**:
   - Gateway đóng vai trò làm Producer đẩy các yêu cầu tổng hợp vào **Redis Streams** (Stream key: `tts:tasks`).
   - Các tiến trình Worker (nằm ngay trong backend hoặc chạy thành các Pod Worker riêng biệt) đóng vai trò Consumer sử dụng **Redis Consumer Groups**.
   - Số lượng công việc chạy đồng thời trong mỗi Worker được khống chế bởi biến môi trường `WORKER_MAX_IN_FLIGHT` (mặc định: `2`).

2. **Chế Độ In-Memory Queue Fallback (Khi không có Redis)**:
   - Hệ thống tự động chuyển sang sử dụng Go Channels nội bộ trong RAM.
   - Thích hợp cho môi trường phát triển cục bộ (Local Development) hoặc triển khai đơn giản 1 replica.

3. **Pool Chuyển Đổi Định Dạng Audio (ffmpeg Transcoder Pool)**:
   - Các file âm thanh tổng hợp đầu ra từ AI Model nếu khác định dạng mong muốn sẽ được đẩy qua công cụ `ffmpeg`.
   - Tiến trình chuyển mã được giới hạn bởi semaphore `TRANSCODE_MAX_CONCURRENCY` (mặc định: `2`) để tránh ngốn toàn bộ CPU/RAM của server.

---

## 🕒 4. Tiến Trình Ngầm (Cron Jobs & Sweepers)

Hệ thống duy trì 2 tiến trình dọn dẹp ngầm (Background Sweeper Jobs) tự động chạy theo chu kỳ để đảm bảo tính ổn định và tiết kiệm tài nguyên lưu trữ:

```
 ┌────────────────────────────────────────────────────────┐
 │            1. Storage Temp Audio Sweeper               │
 │   - Tần suất: Chạy mỗi 1 giờ một lần                   │
 │   - Nhiệm vụ: Xóa các file audio tạm trong           │
 │     `storage/temp/` quá thời hạn                     │
 │     `TEMP_AUDIO_RETENTION_HOURS` (Mặc định: 24 giờ).    │
 └────────────────────────────────────────────────────────┘

 ┌────────────────────────────────────────────────────────┐
 │            2. Database Stale Task Sweeper              │
 │   - Tần suất: Chạy mỗi 5 phút một lần                  │
 │   - Nhiệm vụ: Tim các Task đứng ở trạng thái           │
 │     'pending' hoặc 'processing' quá khoảng thời gian    │
 │     `STALE_CHUNK_AFTER_MINUTES` (Mặc định: 30 phút).    │
 │   - Hành động: Đánh dấu các Task mồ côi này thành      │
 │     'failed' kèm lỗi 'Task timed out / Worker lost'.   │
 └────────────────────────────────────────────────────────┘
```
