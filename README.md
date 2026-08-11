# 🎙️ Better TTS UI Studio - Universal Dynamic Schema-Driven TTS Platform

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Svelte 5](https://img.shields.io/badge/Frontend-Svelte%205-orange.svg)
![Go Chi](https://img.shields.io/badge/Control%20Plane-Go%201.22-00ADD8.svg)
![Python FastAPI](https://img.shields.io/badge/Compute%20Engine-Python%203.10+-green.svg)
![Docker Compose](https://img.shields.io/badge/Deployment-Docker%20Compose-2496ED.svg)

**Better TTS UI Studio** là một nền tảng Web UI & Control Plane hiện đại, hiệu năng cao dành cho các **Kỹ sư AI (AI Engineers)** và nhóm phát triển mô hình tổng hợp giọng nói (Text-to-Speech - TTS). 

Nền tảng được thiết kế theo kiến trúc **Dynamic Schema-Driven (Manifest-Driven)**: Cho phép tích hợp **BẤT KỲ** mô hình AI Speech Synthesis nào (XTTS, GPT-SoVITS, VITS, Fish-Speech, Piper, Kokoro, F5-TTS, VieNeu,...) vào hệ thống giao diện và quản lý chuyên nghiệp **mà không cần viết hay chỉnh sửa bất kỳ dòng mã nguồn Go Gateway hay Svelte Frontend nào!**

---

## 🌟 Tại Sao Chọn Better TTS UI Studio?

Dành cho các AI Engineer muốn đưa mô hình của mình từ notebook/local script ra sản phẩm thực tế:

- ⚡ **Giao Diện Động 100% (Schema-Driven UI)**: Toàn bộ bảng điều khiển tham số (sliders, dropdowns, switches, notice banners, quy tắc nhập liệu) được tự động sinh ra tại thời điểm runtime thông qua file **Manifest (`GET /info`)** do AI Model của bạn khai báo.
- 🪄 **Tự Động Làm Sạch Văn Bản (Regex Rule Engine)**: Khai báo các quy tắc chuẩn hóa văn bản (loại bỏ ký tự đặc biệt, sửa lỗi Unicode Tiếng Việt, xóa gantry,...) trực tiếp trong Manifest để Gateway và Frontend tự xử lý trước khi gửi đến model.
- 🎙️ **Voice Cloning Linh Hoạt**: Định nghĩa các trường thuộc tính mẫu giọng đọc (Accent, Gender, Style, Age,...) hoàn toàn qua JSON Schema.
- 🔒 **Đầy Đủ Tính Năng Enterprise**: Tích hợp sẵn Authentication (JWT HttpOnly Cookies), Phân quyền người dùng (User / Admin), Quản lý lịch sử tổng hợp, Lưu trữ đa nền tảng (Local Disk / AWS S3 / MinIO).
- ⚡ **Xử Lý Hàng Đợi Concurrency Cao**: Backend Go Chi kết hợp Redis Task Queue, cho phép streaming tiến độ realtime qua SSE (Server-Sent Events) và phát âm thanh từng chunk trên giao diện.

---

## 🏗️ Tổng Quan Kiến Trúc Nền Tảng

```
 ┌────────────────────────────────────────────────────────┐
 │                   Frontend (Svelte 5)                  │
 │   - Tự động dựng UI từ Manifest                        │
 │   - Realtime Audio Chunk Player & SSE Streaming        │
 └───────────────────────────┬────────────────────────────┘
                             │ REST API / SSE
 ┌───────────────────────────▼────────────────────────────┐
 │              Control Plane Gateway (Go Chi)            │
 │   - Auth & Session Security (JWT HttpOnly)             │
 │   - Task Queue (Redis Stream) & Job Manager            │
 │   - Multi-tenant Voice & Audio Storage (Local / S3)   │
 └───────────────────────────┬────────────────────────────┘
                             │ HTTP REST / gRPC Core Protocol
 ┌───────────────────────────▼────────────────────────────┐
 │        Dịch Vụ AI Model Của Bạn (Python Core TTS)      │
 │   - PyTorch / ONNX / CUDA Speech Model                 │
 │   - Cung cấp Manifest qua `GET /info`                  │
 └────────────────────────────────────────────────────────┘
```

---

## 🚀 Hướng Dẫn Nhanh Dành Cho AI Engineer Triển Khai Model

### Bước 1: Chuẩn Bị Microservice AI Model Của Bạn

Nền tảng giao tiếp với mô hình AI của bạn thông qua một HTTP REST (hoặc gRPC) Service chuẩn mực. Chúng tôi đã cung cấp sẵn mã nguồn mẫu hoàn chỉnh tại thư mục [`core-tts-example/`](./core-tts-example).

Dịch vụ AI của bạn chỉ cần triển khai 3 endpoint chính:
1. `GET /info` (Bắt buộc): Trả về JSON Manifest định nghĩa thông tin mô hình, bảng tham số UI, quy tắc regex và tính năng Voice Cloning.
2. `GET /voices` (Bắt buộc): Trả về danh sách các giọng đọc có sẵn của mô hình.
3. `POST /tts` (Hoặc gRPC `Synthesize`): Nhận đoạn văn bản + cấu hình tham số và trả về file âm thanh (WAV/MP3/FLAC/OGG).
4. `POST /clone` (Tùy chọn): Nhận file âm thanh mẫu và metadata để đăng ký giọng đọc clone mới.

### Bước 2: Viết Manifest (`GET /info`) Để Dựng Giao Diện

Đây là điểm mấu chốt: **Giao diện người dùng sẽ phản ánh chính xác cấu trúc JSON bạn trả về từ `GET /info`**.

Ví dụ cấu trúc Manifest cơ bản:

```json
{
  "engine": {
    "id": "my-custom-tts",
    "name": "My Custom Neural TTS",
    "version": "1.0.0",
    "description": "Mô hình tổng hợp giọng nói AI tốc độ cao"
  },
  "models": [
    {
      "id": "my-model-v1",
      "name": "Model Tiếng Việt Chuẩn",
      "sample_rate": 24000,
      "supported_formats": ["wav", "mp3"]
    }
  ],
  "ui_schema": {
    "components": [
      {
        "id": "temperature",
        "label": "Độ Sáng Tạo (Temperature)",
        "type": "slider",
        "default": 0.7,
        "min": 0.1,
        "max": 1.0,
        "step": 0.05
      },
      {
        "id": "speed",
        "label": "Tốc Độ Nối (Speed)",
        "type": "slider",
        "default": 1.0,
        "min": 0.5,
        "max": 2.0,
        "step": 0.1
      }
    ]
  }
}
```

👉 Xem chi tiết cấu trúc Manifest và tất cả các component hỗ trợ tại [docs/GATEWAY.md](./docs/GATEWAY.md).

### Bước 3: Khởi Chạy Nền Tảng Với Docker Compose

1. **Clone repository**:
   ```bash
   git clone https://github.com/your-username/better-tts-ui-studio.git
   cd better-tts-ui-studio
   ```

2. **Cấu hình File `.env`**:
   ```bash
   cp .env.example .env
   ```
   Mở file `.env` và điền các khóa bảo mật (có thể sinh bằng lệnh `openssl rand -hex 32`):
   - `SECRET_KEY`: Khóa ký JWT.
   - `POSTGRES_PASSWORD`: Mật khẩu cơ sở dữ liệu.
   - `REDIS_PASSWORD`: Mật khẩu Redis cache/queue.
   - `CORE_ENGINE_URL`: URL tới dịch vụ AI Model của bạn (mặc định trong docker compose là `http://core-engine:8001`).

3. **Khởi chạy hệ thống**:
   ```bash
   docker compose up -d --build
   ```

4. **Truy cập ứng dụng**:
   - 🌐 **Web Studio UI**: [http://localhost:5173](http://localhost:5173)
   - ⚙️ **Control Plane API**: [http://localhost:8000](http://localhost:8000)

Khi bạn thay đổi Manifest hoặc nâng cấp AI Model, chỉ cần gọi endpoint re-sync:
```bash
curl -X POST http://localhost:8000/api/internal/engine/reload
```
Nền tảng sẽ tự động cập nhật UI mới nhất cho toàn bộ người dùng ngay lập tức!

---

## 📚 Hệ Thống Tài Liệu Kỹ Thuật Đầy Đủ

Để tìm hiểu sâu hơn về từng thành phần trong hệ thống, hãy tham khảo các tài liệu chuyên sâu trong thư mục `docs/`:

| File Tài Liệu | Nội Dung Chính |
| :--- | :--- |
| 🗺️ [**docs/SOURCE.md**](./docs/SOURCE.md) | Giải thích cấu trúc mã nguồn toàn bộ dự án (`backend`, `frontend`, `core-tts-example`, `k8s`). |
| 🏗️ [**docs/ARCHITECTURE.md**](./docs/ARCHITECTURE.md) | Kiến trúc hệ thống triển khai production, luồng dữ liệu truyền tải, Task Workers và Cron/Sweeper jobs. |
| 🌐 [**docs/GATEWAY.md**](./docs/GATEWAY.md) | Chi tiết các REST API Endpoints của Gateway và hướng dẫn toàn tập cách viết Manifest (`GET /info`). |
| ⚙️ [**docs/CONFIG.md**](./docs/CONFIG.md) | Hướng dẫn chi tiết thiết lập các biến môi trường (Environment Variables) cho Backend. |
| 🎨 [**docs/FRONTEND.md**](./docs/FRONTEND.md) | Đặc tả kỹ thuật Frontend (Svelte 5 Runes, Web Audio API binary player, SSE streaming, Dynamic UI rendering). |
| 🔧 [**docs/BACKEND.md**](./docs/BACKEND.md) | Đặc tả kỹ thuật Backend (Go Chi, Task Queue Redis Stream, Storage Abstraction, JWT Auth & Database Connection Pool). |

---

## 🛠️ Công Nghệ Sử Dụng

- **Frontend**: Svelte 5 (Runes state management), Vite, FontAwesome 6, Vanilla CSS Glassmorphism.
- **Control Plane Gateway**: Go 1.22, Chi Router, Sonic JSON, PostgreSQL 16 (pgxpool), Redis Stream & Cache.
- **Compute Engine**: Python 3.10+, PyTorch / ONNX / CUDA, FastAPI / Uvicorn / gRPC.
- **Orchestration**: Docker, Docker Compose, Kubernetes (K3s manifests).

---

## 📄 Giấy Phép (License)

Dự án được phát hành theo giấy phép [MIT License](LICENSE).
