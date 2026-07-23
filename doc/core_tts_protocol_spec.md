# Universal TTS Core Protocol Specification (v1.0.0)

Document Version: 1.0.0  
Architecture: Universal Control Plane (Backend) <-> Compute Engine (Core TTS Microservice)

---

## 1. Architecture & Design Philosophy

This specification defines the standard, lightweight RESTful protocol between the **Web Backend Control Plane** (handling Auth, Rate-limiting, User Management, Database History, Chunk Splitting, and Frontend UI) and any **Core TTS Compute Engine Microservice** (handling PyTorch / ONNX / CUDA Model Inference).

### Key Design Principle: Chunk-Based Async REST Architecture
Instead of requiring complex low-level audio streaming protocols (SSE / WebSocket), the Control Plane splits long text into small, natural sentence chunks (<1,000 characters). Each chunk is processed rapidly by the Core TTS Engine via clean, stateless REST requests. This simplifies implementation for AI Engineers while delivering sub-second response times.

```
┌────────────────────────────────────────────────────────────────────────┐
│                        WEB BACKEND CONTROL PLANE                       │
│       (FastAPI / Node.js / Go + Auth + DB + Chunk Splitting + UI)      │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                  LIGHTWEIGHT REST CORE PROTOCOL INTERFACE
     ┌──────────────────────────────┼──────────────────────────────┐
     │ GET /info (Capabilities)     │ POST /synthesize             │
     │ GET /health (GPU Heartbeat)  │ GET  /tasks/{id} (Status)    │
     │ GET /voices (Voice List)     │ DELETE /tasks/{id} (Cancel)  │
     └──────────────────────────────┴──────────────────────────────┘
                                    │
 ┌──────────────────────────────────┴────────────────────────────────────┐
 │                     CORE TTS COMPUTE ENGINE SERVICE                   │
 │        (PyTorch / ONNX / CUDA / VRAM & Model Execution)               │
 └───────────────────────────────────────────────────────────────────────┘
```

---

## 2. Core REST APIs (Mandatory Spec)

Every Core TTS Microservice MUST implement the following 5 RESTful endpoints:

### 2.1 Capabilities & Manifest API
* **Endpoint**: `GET /info` (or `GET /manifest`)
* **Description**: Returns model metadata, supported languages, audio formats, and feature capabilities.

#### Response Example (`application/json`):
```json
{
  "engine_name": "VieNeu-TTS-Core",
  "version": "1.0.0",
  "capabilities": {
    "supports_cloning": true,
    "supports_speed": true,
    "supports_pitch": true
  },
  "audio_formats": ["wav", "mp3"],
  "sample_rates": [22050, 24000, 44100]
}
```

---

### 2.2 Voices Metadata API
* **Endpoint**: `GET /voices`
* **Description**: Returns the list of pre-loaded AI voices available in the Core Engine.

#### Response Example (`application/json`):
```json
[
  {
    "id": "minh_duc",
    "name": "Minh Đức",
    "gender": "male",
    "region": "North",
    "style": "News",
    "language": "vi-VN"
  },
  {
    "id": "hoai_my",
    "name": "Hoài Mỹ",
    "gender": "female",
    "region": "South",
    "style": "Casual",
    "language": "vi-VN"
  }
]
```

---

### 2.3 Healthcheck & Metrics API
* **Endpoint**: `GET /health`
* **Description**: Returns Core service liveness, GPU VRAM utilization, and active task count for Load Balancing.

#### Response Example (`application/json`):
```json
{
  "status": "healthy",
  "gpu_available": true,
  "vram_used_mb": 3200,
  "vram_total_mb": 16384,
  "active_jobs": 1
}
```

---

### 2.4 Core Synthesis API
* **Endpoint**: `POST /synthesize`
* **Description**: Accepts a text chunk and returns synthesized audio bytes or a task ID.

#### Request Payload (`application/json`):
```json
{
  "text": "Xin chào bạn, đây là đoạn văn bản cần đọc.",
  "voice_id": "minh_duc",
  "speed": 1.0,
  "pitch": 0.0,
  "output_format": "wav",
  "ref_voice_id": null
}
```

#### Response:
- **Direct Output**: Binary WAV/MP3 bytes (`Content-Type: audio/wav`).
- **Async Output**: Task ID object:
  ```json
  {
    "task_id": "task_839210",
    "status": "processing"
  }
  ```

---

### 2.5 Task Status & Cancellation API
* **Endpoint**: `GET /tasks/{task_id}` (Status) & `DELETE /tasks/{task_id}` (Cancellation)
* **Description**: Polls task progress or interrupts CUDA inference to release GPU memory.

#### GET Response (`application/json`):
```json
{
  "task_id": "task_839210",
  "status": "done",
  "progress": 100,
  "audio_url": "/audio/task_839210.wav",
  "error": null
}
```

---

## 3. Voice Cloning Extension APIs (Optional)

For Voice Cloning supported engines (e.g. XTTS, GPT-SoVITS, Fish-Speech):

* **`POST /voices/clone`**: Uploads 3-10s audio WAV sample -> extracts Latent Embedding -> returns `voice_id`.
* **`DELETE /voices/{voice_id}`**: Deletes custom voice embedding from storage.
