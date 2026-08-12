# 🏗️ System Architecture & Data Flow (Deployed Architecture & Data Flow)

This document specifies the overall structure of the **Better TTS UI Studio** system when deployed in a Production environment, how components communicate with each other, the Data Flow, the operation mechanism of Task Workers, and Cron/Sweeper background processes.

---

## 🏛️ 1. Deployed Architecture Diagram

When fully deployed (via Docker Compose or Kubernetes), the system is split into 3 independent layers: **Presentation Layer (Frontend)**, **Control Plane Gateway (Backend Go)**, and **Compute Layer (Python AI Engine)**.

```mermaid
flowchart TB
    subgraph Client [" User Browser "]
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

    subgraph Infrastructure [" Database & Memory "]
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
    UI -->|"Download HTML/JS/CSS Static Bundle"| Nginx
    Router <-->|"Write & Query Data"| PG
    Router <-->|"Task Streams & User Cache"| RedisDB
    JobManager <-->|"Store Audio Files & Clone Voices"| StorageEngine
    StorageEngine <-->|"Read/Write Files"| DiskStorage
    
    JobManager <-->|"HTTP / gRPC Synthesis Request"| FastAPI
    JobManager <-->|"gRPC Audio Stream"| gRPCServer

    Router -->|"Trigger Reload Webhook"| Builder
    Builder -->|"Read /api/info to get Manifest"| Router
    Builder -->|"Rebuild index.html Bundle"| Nginx
```

---

## 🔄 2. Detailed Data Flow

### 2.1. Manifest Synchronization Flow (`GET /info` & Webhook Reload)

When the AI Engineer starts or updates the AI model:

```mermaid
sequenceDiagram
    autonumber
    participant Admin as Admin / CI Pipeline
    participant GoGateway as Go Control Plane Gateway
    participant PythonEngine as Python Core TTS Engine
    participant Builder as Frontend Builder (Node.js)
    participant Client as Frontend Browser (Svelte 5)

    GoGateway->>PythonEngine: Startup: GET /info (Get Engine Manifest)
    PythonEngine-->>GoGateway: Return JSON Manifest (Model, UI Schema, Regex Rules)
    GoGateway->>GoGateway: Validate & Cache Manifest in RAM
    
    Admin->>GoGateway: Call POST /api/internal/engine/reload
    GoGateway->>PythonEngine: Send GET /info again to get new Manifest
    GoGateway->>Builder: Call HTTP Webhook (POST FE_BUILDER_URL/reload)
    Builder->>GoGateway: Call GET /api/info to get latest Manifest
    Builder->>Builder: Execute prerender script to rebuild index.html
    GoGateway-->>Client: User accesses/reloads and immediately gets new UI
```

---

### 2.2. Text-to-Speech Processing Flow

This is the main flow when the user generates audio from text:

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

    Client->>Client: Apply Regex Rules to auto-normalize text on Client side
    Client->>Client: Split text into small Chunks (based on max_chars limit)
    Client->>Gateway: POST /api/jobs/init (Create new synthesis Job)
    Gateway->>DB: Create Job record and list of Chunks with 'pending' status
    Gateway-->>Client: Return Job ID and Task ID List

    Client->>Gateway: POST /api/synthesize/{model_id} (Send each Task Chunk)
    Gateway->>Redis: Push Task into Redis Stream Queue `tts:tasks`
    Gateway-->>Client: Return HTTP 202 Accepted (Task ID)

    Client->>Gateway: Connect SSE /api/stream/tasks/{task_id} to listen to progress

    loop Queue Processing Pipeline
        Worker->>Redis: XreadGroup fetch Task to process from Queue
        Worker->>DB: Update Task status to 'processing'
        Worker->>Python: Send HTTP POST /tts (or gRPC Synthesize) with parameters
        Python->>Python: Run PyTorch/CUDA Model to generate Audio Binary (WAV)
        Python-->>Worker: Return Audio Stream Binary
        Worker->>Worker: Convert audio format using ffmpeg (if needed)
        Worker->>Storage: Write audio file to storage (storage/audio/{id}.wav)
        Worker->>DB: Update Task status='completed', save file_path
        Worker->>Gateway: Emit SSE Events: `progress` & `completed`
    end

    Gateway-->>Client: SSE Event notifying Task has completed
    Client->>Gateway: GET /api/tasks/{task_id}/audio (Download audio binary)
    Gateway->>Storage: Read audio file
    Gateway-->>Client: Return Audio File Buffer (Audio/WAV)
    Client->>Client: Web Audio API plays consecutive audio segments on the interface
```

---

### 2.3. Voice Cloning Upload Flow

```mermaid
sequenceDiagram
    autonumber
    participant Client as Frontend WaveformTrimmer
    participant Gateway as Go Gateway
    participant Storage as Storage (Disk / S3)
    participant Python as Python AI Core Engine
    participant DB as PostgreSQL

    Client->>Client: Trim and select the best quality audio segment
    Client->>Gateway: POST /api/clone/upload (File WAV + Metadata: Name, Gender, Accent, Age)
    Gateway->>Gateway: Check file size (must not exceed max_upload_bytes from Manifest)
    Gateway->>Storage: Store reference file to `storage/{user_id}/voice/{id}.wav`
    
    alt If Core Engine requires pre-registration (Register Endpoint)
        Gateway->>Python: Call POST /clone to send reference audio file
        Python-->>Gateway: Return embedding_id or voice_id from Model
    end

    Gateway->>DB: Record new Voice Clone information linked to user_id
    Gateway-->>Client: Return newly created Voice Clone information
    Client->>Client: Update voice list in VoiceSelect Dropdown
```

---

## ⚙️ 3. Task Worker & Concurrency Management

The speech synthesis queue processing system operates in two modes:

1. **Distributed Queue Mode (When Redis is available)**:
   - The Gateway acts as a Producer pushing synthesis requests into **Redis Streams** (Stream key: `tts:tasks`).
   - Worker processes (running within the backend or as separate Worker Pods) act as Consumers using **Redis Consumer Groups**.
   - The number of concurrent jobs in each Worker is limited by the `WORKER_MAX_IN_FLIGHT` environment variable (default: `2`).

2. **In-Memory Queue Fallback Mode (When Redis is unavailable)**:
   - The system automatically switches to using internal Go Channels in RAM.
   - Suitable for local development environments or simple single-replica deployments.

3. **Audio Format Conversion Pool (ffmpeg Transcoder Pool)**:
   - Synthesized audio files from the AI Model, if in a different format than desired, are piped through the `ffmpeg` tool.
   - The transcoding process is limited by the semaphore `TRANSCODE_MAX_CONCURRENCY` (default: `2`) to avoid consuming all server CPU/RAM.

---

## 🕒 4. Background Processes (Cron Jobs & Sweepers)

The system maintains 2 background cleanup processes (Background Sweeper Jobs) that run automatically on a schedule to ensure stability and save storage resources:

```
 ┌────────────────────────────────────────────────────────┐
 │            1. Storage Temp Audio Sweeper               │
 │   - Frequency: Runs once every hour                    │
 │   - Task: Deletes temporary audio files in             │
 │     `storage/temp/` that exceed the retention period    │
 │     `TEMP_AUDIO_RETENTION_HOURS` (Default: 24 hours).   │
 └────────────────────────────────────────────────────────┘

 ┌────────────────────────────────────────────────────────┐
 │            2. Database Stale Task Sweeper              │
 │   - Frequency: Runs once every 5 minutes               │
 │   - Task: Finds Tasks stuck in 'pending' or            │
 │     'processing' status beyond the time period          │
 │     `STALE_CHUNK_AFTER_MINUTES` (Default: 30 minutes).  │
 │   - Action: Marks these orphaned Tasks as               │
 │     'failed' with error 'Task timed out / Worker lost'. │
 └────────────────────────────────────────────────────────┘
```