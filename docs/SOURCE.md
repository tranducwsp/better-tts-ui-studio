# 🗺️ Cấu Trúc Mã Nguồn (Source Code Structure)

Tài liệu này giải thích chi tiết cấu trúc mã nguồn toàn bộ hệ thống **Better TTS UI Studio**, bao gồm vị trí các thư mục, vai trò của từng file và cách tổ chức các tầng xử lý (Layered Architecture).

---

## 📁 Sơ Đồ Cấu Trúc Thư Mục Tổng Thể

```
better-tts-ui-studio/
├── README.md                   # Hướng dẫn chính dành cho AI Engineer
├── docker-compose.yml          # Triển khai Docker Compose toàn bộ stack
├── .env.example                # File mẫu biến môi trường (Sinh tự động từ backend)
├── backend/                    # Control Plane Gateway (Mã nguồn Go 1.22)
├── frontend/                   # Web User Interface (Svelte 5 + Vite)
├── core-tts-example/           # Python AI TTS Compute Engine Mẫu (FastAPI + gRPC)
├── tests/                      # Test suites, fixtures, k6, monitoring, reports
├── k8s/                        # Kubernetes Deployment Manifests (K3s)
└── docs/                       # Thư mục chứa toàn bộ tài liệu kỹ thuật
    ├── SOURCE.md               # [Tài liệu này] Giải thích cấu trúc mã nguồn
    ├── ARCHITECTURE.md         # Kiến trúc hệ thống & Luồng dữ liệu (Data Flow)
    ├── GATEWAY.md              # Chi tiết Endpoints & Hướng dẫn viết Manifest
    ├── CONFIG.md               # Giải thích chi tiết cấu hình biến môi trường
    ├── FRONTEND.md             # Đặc tả kỹ thuật Frontend (Svelte 5 Runes)
    └── BACKEND.md              # Đặc tả kỹ thuật Backend (Go Chi Gateway)
```

---

## 🔧 1. Backend Gateway (`backend/`)

Thư mục `backend/` chứa toàn bộ mã nguồn của Control Plane Gateway được viết bằng ngôn ngữ **Go (Golang 1.22)**.

```
backend/
├── cmd/
│   ├── server/main.go          # Entrypoint chính khởi động backend HTTP server
│   └── gen-env/main.go         # Script tự động đọc config/settings.go để sinh .env.example
├── config/
│   ├── config.go               # Struct Config & hàm LoadConfig() phân tích ENV
│   └── settings.go             # [SINGLE SOURCE OF TRUTH] Khai báo toàn bộ đặc tả biến môi trường
├── app/
│   └── bootstrap.go            # Khởi tạo DB, chạy migration, tạo tài khoản Admin seed
├── router/
│   └── router.go               # Khởi tạo Chi Router, mount middleware và đăng ký tất cả route API
├── middleware/
│   ├── auth.go                 # Middleware xác thực JWT Access Token từ Cookie
│   ├── cors.go                 # Middleware cấu hình CORS cho phép cross-origin
│   ├── logging.go              # Middleware ghi log request HTTP
│   ├── rate_limit.go           # Middleware giới hạn tần suất request (Rate Limiter)
│   ├── concurrency.go          # Middleware giới hạn số lượng request xử lý đồng thời
│   ├── body_limit.go           # Middleware khống chế dung lượng tối đa của HTTP Request Body
│   └── recovery.go             # Middleware bắt panic tránh sập tiến trình
├── handlers/
│   ├── auth.go                 # Đăng ký, Đăng nhập, Logout, Refresh Token, Lấy thông tin user
│   ├── engine_sync.go          # Reload Manifest và kích hoạt webhook rebuild Frontend
│   ├── health.go               # Endpoints /health và /ready cho Load Balancer/K8s
│   ├── history.go              # Quản lý lịch sử tổng hợp của user và admin
│   ├── respond.go              # Helper chuẩn hóa định dạng phản hồi JSON
│   ├── swagger.go              # Phục vụ giao diện OpenAPI / Swagger UI
│   ├── tasks.go                # Tra cứu trạng thái task, hủy task, stream SSE tiến độ, tải audio
│   ├── tts_clone.go            # Đăng ký giọng clone mới, tải file âm thanh tham chiếu lên
│   ├── unified.go              # Endpoint tổng hợp tiếng nói đa năng (/api/synthesize/{model_id})
│   ├── upload.go               # Upload file âm thanh mẫu cho Voice Cloning
│   ├── utils.go                # Trích xuất văn bản từ file (DOCX, PDF, TXT)
│   └── preset_voices.go        # Danh sách & bộ đệm giọng mẫu (preset) của hệ thống
├── synth/
│   ├── pipeline.go             # Luồng tổng hợp tiếng nói: Phân đoạn văn bản (chunking) & điều phối
│   └── local.go                # Tương tác với công cụ ffmpeg để chuyển đổi định dạng âm thanh
├── queue/
│   └── job_queue.go            # Hàng đợi công việc (Task Queue) dựa trên Redis Streams
├── storage/
│   ├── store.go                # Interface Store định nghĩa các thao tác lưu trữ
│   ├── local.go                # Triển khai lưu trữ file trên ổ đĩa cục bộ (Local Disk)
│   ├── s3.go                   # Triển khai lưu trữ file trên AWS S3 / MinIO
│   ├── paths.go                # Helper tạo đường dẫn thư mục chuẩn hóa
│   └── sweeper.go              # Tiến trình dọn dẹp các file âm thanh tạm bị hết hạn
├── database/
│   ├── db.go                   # Khởi tạo Connection Pool kết nối PostgreSQL (pgxpool)
│   ├── queries.go              # Thực thi các câu lệnh SQL (CRUD User, Job, Voice, Task)
│   ├── migrations.go           # Tự động thực thi Migration bảng DB khi khởi động
│   └── sweeper.go              # Tiến trình background dọn dẹp các task bị treo/mồ côi (stale tasks)
├── client/
│   ├── engine_client_http.go   # HTTP Client gọi API Manifest và Synthesize tới Python Core TTS Engine
│   └── engine_client_grpc.go   # gRPC Client kết nối tới Python Core TTS Engine qua Protocol Buffers
├── types/
│   ├── manifest.go             # Định nghĩa Go Struct cho Engine Manifest & UI Schema
│   └── validate.go             # Hàm kiểm tra tính hợp lệ của Manifest
├── security/
│   └── auth.go                 # Hàm băm mật khẩu (Bcrypt) và Tạo/Giải mã JWT Tokens
└── go.mod                      # Khai báo các phụ thuộc thư viện Go
```

---

## 🧪 3. Test Suites (`backend/tests/`, `frontend/tests/`, `tests/`)

Repository tổ chức test theo ba cấp:

```
tests/                          # Root-level test assets
├── load/                       # K6 load test scripts (smoke, load, stress, soak)
├── monitoring/                 # Python monitoring scripts
├── reports/                    # HTML/JSON metrics reports
└── fixtures/                   # Shared test fixtures (dùng chung Go + frontend)

backend/tests/                  # Backend Go test suites
├── unit/                       # Unit tests (không cần Redis/DB, chạy độc lập)
│   ├── handlers/               # Handler tests
│   ├── middleware/              # Middleware tests
│   ├── presetvoicecache/        # Preset voice cache tests
│   ├── security/               # Security/auth tests
│   ├── state/                  # State management tests (no-Redis variants)
│   ├── storage/                # Storage tests
│   └── types/                  # Type validation tests
├── integration/                # Integration tests (cần Redis/DB)
│   ├── db/                     # Database integration tests
│   ├── middleware/             # Redis middleware tests
│   └── state/                  # State integration tests (Redis-backed)
├── contract/                   # Contract/parity tests (Go + frontend đồng bộ)
│   ├── config/                 # Config settings tests
│   ├── db/                     # DB schema parity tests
│   ├── manifest/               # Manifest documentation tests
│   ├── parity/                 # Capability resolution parity tests
│   └── schema/                 # Schema parity tests
└── testsupport/                # Test helpers (RepoRoot, Path)

frontend/tests/                 # Frontend test suites
└── unit/                       # Unit tests (vitest)
    ├── audioSpec.test.ts       # Audio spec resolution tests
    ├── capabilities.test.ts    # Capability resolution tests
    └── ranges.test.ts          # Range resolution tests
```

### Quy tắc tổ chức test

- **Không đặt test file cạnh source code.** Mọi test file phải nằm trong thư mục `tests/` tương ứng.
- Backend test package dùng external package (`package xxx_test`) để đảm bảo test chỉ chạm vào exported API.
- Integration test cần Redis gated bằng `TEST_REDIS_ADDR` env var; không có Redis thì skip.
- `tests/fixtures/` chứa dữ liệu test dùng chung giữa Go và frontend — nếu thay đổi fixture, cả hai suite phải cùng pass.
- Frontend test chạy bằng vitest, cấu hình trong `frontend/vitest.config.ts`.

---

## 🎨 2. Frontend Studio (`frontend/`)

Thư mục `frontend/` chứa giao diện người dùng Web Studio được xây dựng bằng **Svelte 5 (Runes)** và **Vite**.

```
frontend/
├── index.html                  # HTML entrypoint cho trang web
├── vite.config.ts              # Cấu hình trình đóng gói Vite
├── svelte.config.js            # Cấu hình Svelte compiler
├── package.json                # Danh sách thư viện phụ thuộc (Svelte 5, FontAwesome,...)
├── public/                     # Các tệp tĩnh (Fonts Inter, FontAwesome Webfonts, Favicon)
├── scripts/
│   ├── prerender.js            # Script tải Manifest từ Backend và prerender trang HTML
│   └── builder_server.js       # Server lắng nghe Webhook reload từ Backend để dựng lại bundle
└── src/
    ├── main.ts                 # Entrypoint khởi tạo ứng dụng Svelte
    ├── App.svelte              # Component gốc chứa layout chính và chuyển đổi màn hình (Auth/Studio)
    ├── app.css                 # File định kiểu CSS toàn cục (Design System & Glassmorphic variables)
    └── lib/
        ├── api.ts              # HTTP Client giao tiếp với Backend Gateway (Fetch wrapper với Cookie Credentials)
        ├── capabilities.ts     # Phân tích capabilities và kiểm tra tính năng hỗ trợ của Model
        ├── ranges.ts           # Xử lý tính toán giá trị tham số slider
        ├── textLimits.ts       # Kiểm tra giới hạn số ký tự/từ của đoạn văn bản
        ├── audioWav.ts         # Xử lý decode/encode file âm thanh WAV ở trình duyệt
        ├── toast.svelte.ts     # Hệ thống thông báo Toasts reactive dùng Svelte 5 $state
        └── components/
            ├── Header.svelte            # Thanh điều hướng trên cùng (User info, Admin link, Reload manifest)
            ├── TextInputPanel.svelte    # Panel nhập liệu văn bản, công cụ Tìm/Thay thế & Tự động format
            ├── GenericEnginePanel.svelte# Panel tự động sinh các Slider, Dropdown tham số từ Manifest UI Schema
            ├── StreamingPanel.svelte    # Bảng phát âm thanh realtime từng chunk và thanh tiến độ SSE
            ├── VoiceSelect.svelte       # Dropdown chọn giọng đọc (System voices & User clone voices)
            ├── CreateVoiceModal.svelte  # Modal tạo giọng clone mới (Upload file & Nhập metadata)
            ├── WaveformTrimmer.svelte   # Component hiển thị dạng sóng (Waveform) và cắt file âm thanh mẫu
            ├── HistoryModal.svelte      # Modal xem danh sách lịch sử các đoạn audio đã tổng hợp
            ├── AuthModal.svelte         # Modal Đăng nhập / Đăng ký tài khoản
            └── AdminModal.svelte        # Modal dành cho Admin duyệt tài khoản người dùng
```

---

## 🐍 3. Compute Engine Mẫu (`core-tts-example/`)

Thư mục `core-tts-example/` là một mô hình ví dụ hoàn chỉnh bằng **Python (FastAPI + PyTorch/gRPC)** thể hiện cách AI Engineer kết nối mô hình của mình vào nền tảng.

```
core-tts-example/
├── main.py                     # Entrypoint khởi chạy FastAPI REST Server (Trả về Manifest và xử lý tổng hợp)
├── grpc_server.py              # Server gRPC phục vụ tổng hợp tiếng nói tốc độ cao
├── schemas.py                  # Pydantic Schemas cho Manifest, Engine Info, Synthesis Requests
├── Dockerfile                  # Containerize dịch vụ Python AI Model
├── requirements.txt            # Thư viện Python (FastAPI, uvicorn, grpcio, torch, pydantic)
├── proto/
│   ├── tts.proto               # File định nghĩa gRPC Protocol Buffers cho TTS Service
│   ├── tts_pb2.py              # File mã nguồn Python sinh ra từ Protobuf
│   └── tts_pb2_grpc.py         # File gRPC Stubs
├── utils/
│   └── audio_utils.py          # Helper tạo dữ liệu âm thanh tín hiệu thử nghiệm (Sine wave WAV generator)
└── scripts/
    ├── build_proto.sh          # Script biên dịch file .proto thành mã Python
    └── run_example.sh          # Script khởi chạy nhanh dịch vụ Python
```

---

## 🐳 4. Deployment & Configuration (`k8s/`, `docker-compose.yml`)

- **`docker-compose.yml`**: Định nghĩa 5 container chính hoạt động cùng nhau:
  1. `backend`: Gateway Go Chi (cổng `8000`).
  2. `frontend`: Web Server Nginx phục vụ Svelte 5 UI static bundle (cổng `5173`).
  3. `frontend-builder`: Tiến trình Node.js lắng nghe webhook để rebuild prerender HTML bundle (cổng `3001`).
  4. `postgres`: Cơ sở dữ liệu PostgreSQL 16 (cổng `5432` nội bộ).
  5. `redis`: Redis server lưu trữ Task Streams và Cache (cổng `6379` nội bộ).
  6. `core-engine`: Dịch vụ AI Model mẫu (cổng `8001` nội bộ).
- **`k8s/`**: Chứa các file Kubernetes Deployment, Service, ConfigMap, StatefulSet phục vụ triển khai sản phẩm lên cụm Kubernetes (K3s).
