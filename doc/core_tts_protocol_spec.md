# Universal TTS Core Protocol Specification (v1.0.0)

Document Version: 1.0.0  
Architecture: Universal Control Plane (Backend) <-> Compute Engine (Core TTS Microservice)

---

## 1. Architecture & Design Philosophy

This specification defines the standard, lightweight RESTful protocol between the **Web Backend Control Plane** (handling Auth, Rate-limiting, User Management, Database History, Chunk Splitting, and Dynamic UI Rendering) and any **Core TTS Compute Engine Microservice** (handling PyTorch / ONNX / CUDA Model Inference).

### Key Design Principle: Schema-Driven & Chunk-Based REST Architecture
Instead of hardcoding engine features in the UI or Control Plane, the **Core TTS Compute Engine** exposes a dynamic Manifest via `GET /info`. The Control Plane automatically adapts UI controls, text normalization rules, and voice metadata creation forms dynamically based on this manifest.

```
┌────────────────────────────────────────────────────────────────────────┐
│                        WEB BACKEND CONTROL PLANE                       │
│      (Go Chi Engine Gateway + Auth + DB + Chunk Splitting + Svelte UI) │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                  LIGHTWEIGHT REST CORE PROTOCOL INTERFACE
     ┌──────────────────────────────┼──────────────────────────────┐
     │ GET /info (UI Manifest)      │ POST /synthesize             │
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

Every Core TTS Microservice MUST implement the following RESTful endpoints:

### 2.1 Capabilities & Manifest API
* **Endpoint**: `GET /info` (or `GET /manifest`)
* **Description**: Returns model metadata, supported modes, audio specifications, and UI configuration schema.

#### Response Example (`application/json`):
```json
{
  "engine_id": "core-engine-v1",
  "engine_name": "Universal Core AI Engine",
  "version": "1.0.0",
  "provider": "Universal AI Platform",
  "supported_modes": [
    {
      "id": "fast",
      "name": "Fast Streaming Engine",
      "description": "Low latency streaming TTS",
      "supports_preset_voices": true,
      "supports_cloning": false,
      "supports_voice_saving": false,
      "supports_streaming": true
    },
    {
      "id": "clone",
      "name": "Voice Cloning Engine",
      "description": "Reference audio speaker cloning",
      "supports_preset_voices": true,
      "supports_cloning": true,
      "supports_voice_saving": true,
      "supports_streaming": true
    }
  ],
  "capabilities": {
    "supports_preset_voices": true,
    "supports_cloning": true,
    "supports_streaming": true,
    "supports_speed": true,
    "supports_pitch": false,
    "supports_emotion": false
  },
  "constraints": {
    "max_text_length": 3000,
    "speed_range": { "min": 0.5, "max": 2.0, "default": 1.0, "step": 0.1 }
  },
  "audio_spec": {
    "supported_formats": ["wav", "mp3"],
    "supported_sample_rates": [16000, 22050, 24000, 44100],
    "default_format": "wav",
    "default_sample_rate": 24000
  },
  "ui_schema": {
    "input_panel": {
      "file_serve": true,
      "closeable": false,
      "find_mode": "expert",
      "replace_tool": true,
      "enable_chunk_box": true,
      "auto_format": [
        { "find": "\\r\\n", "replace": "\n" },
        { "find": "\\n{3,}", "replace": "\n\n" },
        { "find": "\\u00D0", "replace": "Đ" },
        { "find": "([a-zA-ZÀ-ỹ])\\-([a-zA-ZÀ-ỹ])", "replace": "$1 $2" },
        { "find": "[⁰¹²³⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉]", "replace": "" },
        { "find": "[^a-zA-Z0-9 \\n\\t\\r.,?!;:\\-\"'()\\[\\]%/“”‘’À-ỹ]", "replace": "" }
      ]
    },
    "model_sort": ["fast", "standard", "clone"],
    "option_panel": {
      "clone": {
        "voice_type": "select",
        "speed_type": "slider",
        "voice_metadata_schema": [
          { "key": "name", "label": "Tên giọng mẫu", "type": "text", "required": true },
          { "key": "gender", "label": "Giới tính", "type": "select", "options": ["Nam", "Nữ", "Khác"] },
          { "key": "region", "label": "Vùng miền", "type": "select", "options": ["Miền Bắc", "Miền Nam", "Miền Trung", "Khác"] }
        ]
      }
    }
  }
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
  }
]
```

---

### 2.3 Healthcheck & Metrics API
* **Endpoint**: `GET /health`
* **Description**: Returns Core service liveness, GPU VRAM utilization, and active task count.

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

---

### 2.5 Task Status & Cancellation API
* **Endpoint**: `GET /tasks/{task_id}` & `DELETE /tasks/{task_id}`
* **Description**: Polls task progress or interrupts CUDA inference.

---

## 3. Voice Cloning Extension APIs (Optional)

For Voice Cloning supported engines (e.g. XTTS, GPT-SoVITS, Fish-Speech):

* **`POST /voices/clone`**: Uploads reference audio WAV sample -> extracts Latent Embedding -> returns `voice_id`.
* **`DELETE /voices/{voice_id}`**: Deletes custom voice embedding from storage.
