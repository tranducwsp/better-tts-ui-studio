# 🗺️ Source Code Structure

This document explains the detailed source code structure of the entire **Better TTS UI Studio** system, including directory locations, the role of each file, and the organization of processing layers (Layered Architecture).

---

## 📁 Overall Directory Structure Diagram

```
better-tts-ui-studio/
├── README.md                   # Main guide for AI Engineers
├── docker-compose.yml          # Docker Compose deployment for the entire stack
├── .env.example                # Environment variable sample file (Auto-generated from backend)
├── backend/                    # Control Plane Gateway (Go 1.22 Source Code)
├── frontend/                   # Web User Interface (Svelte 5 + Vite)
├── core-tts-example/           # Python AI TTS Compute Engine Example (FastAPI + gRPC)
├── k8s/                        # Kubernetes Deployment Manifests (K3s)
└── docs/                       # Directory containing all technical documentation
    ├── SOURCE.md               # [This document] Explains the source code structure
    ├── ARCHITECTURE.md         # System Architecture & Data Flow
    ├── GATEWAY.md              # Endpoint Details & Manifest Writing Guide
    ├── CONFIG.md               # Detailed environment variable configuration explanation
    ├── FRONTEND.md             # Frontend Technical Specification (Svelte 5 Runes)
    └── BACKEND.md              # Backend Technical Specification (Go Chi Gateway)
```

---

## 🔧 1. Backend Gateway (`backend/`)

The `backend/` directory contains all source code for the Control Plane Gateway written in **Go (Golang 1.22)**.

```
backend/
├── cmd/
│   ├── server/main.go          # Main entrypoint to start the backend HTTP server
│   └── gen-env/main.go         # Script that auto-reads config/settings.go to generate .env.example
├── config/
│   ├── config.go               # Config Struct & LoadConfig() function that parses ENV
│   └── settings.go             # [SINGLE SOURCE OF TRUTH] Declares all environment variable specifications
├── app/
│   └── bootstrap.go            # Initialize DB, run migrations, create seed Admin account
├── router/
│   └── router.go               # Initialize Chi Router, mount middleware, and register all API routes
├── middleware/
│   ├── auth.go                 # Middleware for JWT Access Token authentication from Cookie
│   ├── cors.go                 # Middleware for CORS configuration allowing cross-origin
│   ├── logging.go              # Middleware for HTTP request logging
│   ├── rate_limit.go           # Middleware for request rate limiting (Rate Limiter)
│   ├── concurrency.go          # Middleware for limiting concurrent request processing count
│   ├── body_limit.go           # Middleware for capping maximum HTTP Request Body size
│   └── recovery.go             # Middleware to catch panics and prevent process crashes
├── handlers/
│   ├── auth.go                 # Register, Login, Logout, Refresh Token, Get user info
│   ├── engine_sync.go          # Reload Manifest and trigger webhook to rebuild Frontend
│   ├── health.go               # /health and /ready endpoints for Load Balancer/K8s
│   ├── history.go              # Manage user and admin synthesis history
│   ├── respond.go              # Helper to standardize JSON response format
│   ├── swagger.go              # Serve OpenAPI / Swagger UI interface
│   ├── tasks.go                # Look up task status, cancel task, SSE progress stream, download audio
│   ├── tts_clone.go            # Register new clone voice, upload reference audio file
│   ├── unified.go              # Universal speech synthesis endpoint (/api/synthesize/{model_id})
│   ├── upload.go               # Upload sample audio file for Voice Cloning
│   ├── utils.go                # Extract text from files (DOCX, PDF, TXT)
│   └── voice_cache.go          # In-memory cache for voice information
├── synth/
│   ├── pipeline.go             # Speech synthesis flow: Text chunking & coordination
│   └── local.go                # Interaction with ffmpeg tool for audio format conversion
├── queue/
│   └── redis_stream.go         # Job Queue (Task Queue) based on Redis Streams
├── storage/
│   ├── store.go                # Store Interface defining storage operations
│   ├── local.go                # Implementation for storing files on local disk
│   ├── s3.go                   # Implementation for storing files on AWS S3 / MinIO
│   ├── paths.go                # Helper to create standardized directory paths
│   └── sweeper.go              # Background process to clean up expired temporary audio files
├── database/
│   ├── db.go                   # Initialize PostgreSQL Connection Pool (pgxpool)
│   ├── queries.go              # Execute SQL statements (CRUD User, Job, Voice, Task)
│   ├── migrations.go           # Automatically execute DB table Migrations on startup
│   └── sweeper.go              # Background process to clean up stuck/orphaned tasks (stale tasks)
├── client/
│   ├── core_tts.go             # HTTP Client calling Manifest and Synthesize API to Python Core TTS Engine
│   └── grpc_tts.go             # gRPC Client connecting to Python Core TTS Engine via Protocol Buffers
├── types/
│   ├── manifest.go             # Define Go Structs for Engine Manifest & UI Schema
│   └── validate.go             # Function to check Manifest validity
├── security/
│   └── auth.go                 # Password hashing (Bcrypt) and Create/Decode JWT Tokens
└── go.mod                      # Declare Go library dependencies
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

The `frontend/` directory contains the Web Studio user interface built with **Svelte 5 (Runes)** and **Vite**.

```
frontend/
├── index.html                  # HTML entrypoint for the website
├── vite.config.ts              # Vite bundler configuration
├── svelte.config.js            # Svelte compiler configuration
├── package.json                # List of dependencies (Svelte 5, FontAwesome, etc.)
├── public/                     # Static files (Inter Fonts, FontAwesome Webfonts, Favicon)
├── scripts/
│   ├── prerender.js            # Script to fetch Manifest from Backend and prerender the HTML page
│   └── builder_server.js       # Server that listens for Webhook reload from Backend to rebuild the bundle
└── src/
    ├── main.ts                 # Entrypoint to initialize the Svelte application
    ├── App.svelte              # Root component containing main layout and screen switching (Auth/Studio)
    ├── app.css                 # Global CSS stylesheet (Design System & Glassmorphic variables)
    └── lib/
        ├── api.ts              # HTTP Client communicating with Backend Gateway (Fetch wrapper with Cookie Credentials)
        ├── capabilities.ts     # Analyze capabilities and check Model feature support
        ├── ranges.ts           # Handle slider parameter value calculations
        ├── textLimits.ts       # Check character/word limits of text segments
        ├── audioWav.ts         # Handle WAV audio file decode/encode in the browser
        ├── toast.svelte.ts     # Reactive Toast notification system using Svelte 5 $state
        └── components/
            ├── Header.svelte            # Top navigation bar (User info, Admin link, Reload manifest)
            ├── TextInputPanel.svelte    # Text input panel, Find/Replace tool & Auto-format
            ├── GenericEnginePanel.svelte# Panel auto-generating Sliders, Dropdowns from Manifest UI Schema
            ├── StreamingPanel.svelte    # Realtime per-chunk audio playback panel and SSE progress bar
            ├── VoiceSelect.svelte       # Voice selection dropdown (System voices & User clone voices)
            ├── CreateVoiceModal.svelte  # Modal to create new clone voice (Upload file & Enter metadata)
            ├── WaveformTrimmer.svelte   # Component displaying waveform and trimming sample audio file
            ├── HistoryModal.svelte      # Modal to view the history list of synthesized audio segments
            ├── AuthModal.svelte         # Login / Register account modal
            └── AdminModal.svelte        # Modal for Admin to approve user accounts
```

---

## 🐍 3. Compute Engine Example (`core-tts-example/`)

The `core-tts-example/` directory is a complete example model in **Python (FastAPI + PyTorch/gRPC)** demonstrating how an AI Engineer connects their model to the platform.

```
core-tts-example/
├── main.py                     # Entrypoint to start FastAPI REST Server (Returns Manifest and handles synthesis)
├── grpc_server.py              # gRPC Server serving high-speed speech synthesis
├── schemas.py                  # Pydantic Schemas for Manifest, Engine Info, Synthesis Requests
├── Dockerfile                  # Containerize the Python AI Model service
├── requirements.txt            # Python libraries (FastAPI, uvicorn, grpcio, torch, pydantic)
├── proto/
│   ├── tts.proto               # gRPC Protocol Buffers definition file for TTS Service
│   ├── tts_pb2.py              # Python source code generated from Protobuf
│   └── tts_pb2_grpc.py         # gRPC Stubs
├── utils/
│   └── audio_utils.py          # Helper to create test audio signal data (Sine wave WAV generator)
└── scripts/
    ├── build_proto.sh          # Script to compile .proto files into Python code
    └── run_example.sh          # Quick launch script for the Python service
```

---

## 🐳 4. Deployment & Configuration (`k8s/`, `docker-compose.yml`)

- **`docker-compose.yml`**: Defines 6 main containers working together:
  1. `backend`: Go Chi Gateway (port `8000`).
  2. `frontend`: Nginx Web Server serving Svelte 5 UI static bundle (port `5173`).
  3. `frontend-builder`: Node.js process listening for webhook to rebuild the prerender HTML bundle (port `3001`).
  4. `postgres`: PostgreSQL 16 database (internal port `5432`).
  5. `redis`: Redis server for Task Streams storage and Cache (internal port `6379`).
  6. `core-engine`: Example AI Model service (internal port `8001`).
- **`k8s/`**: Contains Kubernetes Deployment, Service, ConfigMap, StatefulSet files for deploying the product to a Kubernetes cluster (K3s).