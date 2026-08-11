# Ghi Chú Đề Xuất Refactor Tên File Backend

> **Mục tiêu**: Đổi tên các file nguồn thể hiện công nghệ (HOW) sang tên thể hiện đúng nghiệp vụ/mục đích (WHAT/WHY) theo nguyên tắc Clean Code & Domain-Driven Design (DDD).

---

## 📋 Danh Sách Các File Cần Đổi Tên

| File Hiện Tại (Công nghệ) | File Đề Xuất (Nghiệp vụ) | Lý Do & Ý Nghĩa Nghiệp Vụ |
| :--- | :--- | :--- |
| [`backend/queue/redis_stream.go`](file:///home/amora/better-tts-ui-studio/backend/queue/redis_stream.go) | `backend/queue/job_queue.go` | Thể hiện hàng chờ xử lý các job/task TTS. Tránh phụ thuộc tên file vào công nghệ Redis Stream (dễ dàng đổi sang NATS/RabbitMQ về sau). |
| [`backend/client/grpc_tts.go`](file:///home/amora/better-tts-ui-studio/backend/client/grpc_tts.go) | `backend/client/engine_client.go` | Client giao tiếp với TTS Engine lõi. Tránh gắn tên file với giao thức gRPC. |
| [`backend/handlers/voice_cache.go`](file:///home/amora/better-tts-ui-studio/backend/handlers/voice_cache.go) | `backend/handlers/preset_voices.go` | Quản lý danh sách & bộ đệm giọng mẫu (preset) của hệ thống thay vì nói về cơ chế cache. |

---

## 📌 Các Nguyên Tắc Cần Chú Ý Khi Refactor Lại
1. **Giữ nguyên package name**: Khi đổi tên file `.go`, tên gói (`package queue`, `package client`, `package handlers`) vẫn giữ nguyên.
2. **Cập nhật Import & Reference**: Đảm bảo cập nhật lại tên file nếu có các file test tương ứng (ví dụ: `voice_cache_test.go` -> `preset_voices_test.go`).
3. **Giữ nguyên Interface contract**: Chỉ thay đổi vị trí/tên file lưu trữ, giữ nguyên struct/interface exported để không làm hỏng code đang gọi ở `router` hay `handlers`.

---

## 🧹 Đề Xuất Dọn Dẹp Comment Code Lịch Sử (Legacy Comments)

> **Vấn đề**: Nhiều file trong `backend/` chứa các đoạn comment dài giải thích về "code cũ", "bản cũ làm gì", "trước đây..." (ví dụ: giải thích lý do tại sao hàm cũ ngốn RAM, route cũ bị thiếu security, v.v.).
> **Quy tắc Clean Code**: Comment nên giải thích **tại sao đoạn code HIỆN TẠI hoạt động như thế này** (Why/Current Behavior) thay vì đóng vai trò làm nhật ký thay đổi (Change Log). Lịch sử thay đổi code là nhiệm vụ của **Git Commit History**.

### Các File Tiêu Biểu Chứa Nhiều Comment "Trực/Trước Đây":
1. **[`backend/handlers/utils.go`](file:///home/amora/better-tts-ui-studio/backend/handlers/utils.go)**: Nhiều comment so sánh hàm bóc chữ PDF/DOCX hiện tại với bản cũ ("Trước đây ExtractText đọc CẢ upload...", "Bản cũ copy nguyên input...").
2. **[`backend/handlers/voice_cache.go`](file:///home/amora/better-tts-ui-studio/backend/handlers/voice_cache.go)**: Comment giải thích logic cache so sánh với bản trước đó.
3. **[`backend/handlers/engine_sync.go`](file:///home/amora/better-tts-ui-studio/backend/handlers/engine_sync.go)**: Comment kể lại lịch sử route `/internal/engine/reload` từng mở public gây lỗi ra sao.
4. **[`backend/middleware/ratelimit.go`](file:///home/amora/better-tts-ui-studio/backend/middleware/ratelimit.go)** & **[`auth.go`](file:///home/amora/better-tts-ui-studio/backend/middleware/auth.go)**: Nhiều comment so sánh cơ chế rate-limit / session cũ và mới.

### Đề Xuất Hướng Xử Lý:
- Giữ lại docstring giải thích logic **hiện tại**.
- Lược bỏ bớt các đoạn nhắc về "bản cũ / trước đây" để code ngắn gọn, súc tích và đỡ rối mắt hơn.

---

## 🧪 Phân Tích & Lưu Ý Về File Unit Test (`*_test.go`) & Release Build

> **Cảm nhận**: Đặt các file `*_test.go` nằm rải rác cùng thư mục với code chạy (như `handlers/`, `middleware/`) có thể tạo cảm giác rối mắt.

### 1. Về Việc Build / Release (Đóng gói sản phẩm)
- **Không hề ảnh hưởng đến dung lượng hay hiệu năng sản phẩm**: 
  Khi bạn chạy `go build`, trình biên dịch của Go **hoàn toàn bỏ qua (ignore)** tất cả các file có đuôi `*_test.go`. File binary tạo ra hoàn toàn sạch sẽ, không chứa bất kỳ dòng code test nào.
- Docker Image release (qua `Dockerfile` dạng multi-stage) chỉ copy duy nhất file binary đã build, nên artifact release rất nhẹ và an toàn.

---

## 📚 Dọn Dẹp Các File README Dư Thừa

> **Vấn đề**: Dự án đã có hai file tài liệu chính chủ ở thư mục gốc: [`README.md`](file:///home/amora/better-tts-ui-studio/README.md) và [`TECHNICAL_README.md`](file:///home/amora/better-tts-ui-studio/TECHNICAL_README.md), cùng với thư mục [`docs/`](file:///home/amora/better-tts-ui-studio/docs) chứa toàn bộ tài liệu chi tiết. Việc tồn tại các file `README.md` nhỏ lẻ ở các thư mục con (`frontend/`, `test/`, `core-tts-example/`...) dễ gây phân tán thông tin và lỗi thời khi cập nhật.

### Các File README Dư Thừa Cần Xoá Hoặc Gom Về `docs/`:
1. **[`frontend/README.md`](file:///home/amora/better-tts-ui-studio/frontend/README.md)**: Nên xoá hoặc gộp nội dung vào [`docs/frontend_overview.md`](file:///home/amora/better-tts-ui-studio/docs/frontend_overview.md).
2. **[`test/README.md`](file:///home/amora/better-tts-ui-studio/test/README.md)**: Hướng dẫn test nên gộp chung vào [`TECHNICAL_README.md`](file:///home/amora/better-tts-ui-studio/TECHNICAL_README.md) mục Testing.
3. **[`core-tts-example/README.md`](file:///home/amora/better-tts-ui-studio/docs/examples/README.md)**: Gộp vào hướng dẫn tích hợp engine trong [`docs/ENGINEER_INTEGRATION_GUIDE.md`](file:///home/amora/better-tts-ui-studio/docs/ENGINEER_INTEGRATION_GUIDE.md).

### 💡 Hướng Xử Lý:
- **Single Source of Truth**: Tập trung 100% tài liệu vào thư mục gốc `README.md`, `TECHNICAL_README.md` và thư mục `docs/`.
- Xoá các file `README.md` con để tránh việc dev sau sửa một chỗ mà quên cập nhật chỗ khác.

### 2. Về Tổ Chức Codebase Hiện Tại Của Dự Án
- Dự án đã gom **32 file test** vào thư mục riêng: [`backend/tests/`](file:///home/amora/better-tts-ui-studio/backend/tests).
- **Tuy nhiên**, đúng như bạn phát hiện, vẫn còn một số file `*_test.go` nằm rải rác bên trong các thư mục module:
  - [`backend/state/task_cancel_test.go`](file:///home/amora/better-tts-ui-studio/backend/state/task_cancel_test.go)
  - [`backend/config/settings_test.go`](file:///home/amora/better-tts-ui-studio/backend/config/settings_test.go)
  - [`backend/handlers/voice_cache_test.go`](file:///home/amora/better-tts-ui-studio/backend/handlers/voice_cache_test.go)

### 💡 Đề Xuất Kế Hoạch Di Chuyển (Refactor Test Files):
- Di chuyển nốt 3 file test trên về chung thư mục [`backend/tests/`](file:///home/amora/better-tts-ui-studio/backend/tests) để gom toàn bộ 100% test về một nơi duy nhất.
- Giúp các thư mục `state/`, `config/`, `handlers/` hoàn toàn sạch sẽ, không còn chứa bất kỳ file `*_test.go` nào nữa.

---

## 🌐 Tách Vai Trò Nginx Trong Frontend Container

> **Phân tích**: Hiện tại Nginx ở [`frontend/nginx.conf`](file:///home/amora/better-tts-ui-studio/frontend/nginx.conf) đang ôm hai nhiệm vụ: (1) Serve file tĩnh React/Vite và (2) Proxy `/api/` & `/storage/` sang Backend.

### 💡 Hướng Cải Tiến:
- **Đơn giản hóa Nginx trong Frontend**: Tách Nginx ở FE ra chỉ tập trung **duy nhất vào nhiệm vụ serve file tĩnh** (`index.html`, `assets/`, `fonts/`), loại bỏ toàn bộ các block `location /api/` và `resolver 127.0.0.11`.
- **Môi trường Deploy**:
  - **Docker Compose (Local/Standalone)**: Giữ Nginx nhẹ nhàng ở FE serve file tĩnh, FE gọi API trực tiếp qua domain/port Backend.
  - **Kubernetes (Production)**: Sử dụng Ingress Controller chuẩn (như **Traefik Ingress** hoặc **Nginx Ingress**) đứng ở cấp K8s Cluster để điều hướng đường dẫn `/api` sang Backend Service và `/` sang Frontend Service.


