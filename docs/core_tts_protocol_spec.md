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
      "capabilities": {}
    },
    {
      "id": "clone",
      "name": "Voice Cloning Engine",
      "description": "Reference audio speaker cloning",
      "capabilities": { "supports_cloning": true, "supports_voice_saving": true }
    },
    {
      "id": "emotion_v2",
      "name": "Emotion & Style",
      "description": "Dynamic prosody control",
      "capabilities": { "supports_pitch": true, "supports_emotion": true }
    }
  ],
  "capabilities": {
    "supports_preset_voices": true,
    "supports_cloning": false,
    "supports_voice_saving": false,
    "supports_streaming": true,
    "supports_speed": true,
    "supports_pitch": false,
    "supports_emotion": false,
    "supports_ssml": false
  },
  "constraints": {
    "max_text_length": 3000,
    "speed_range": { "min": 0.5, "max": 2.0, "default": 1.0, "step": 0.1 },
    "pitch_range": { "min": -10.0, "max": 10.0, "default": 0.0, "step": 0.5 },
    "supported_emotions": ["neutral", "happy", "sad"],
    "chunking": {
      "max_chunk_size": 1000,
      "delimiters": ["(?<=\\.\\s*\\n)", "(?<=[.!?]\\s+)"]
    }
  },
  "audio_spec": {
    "supported_formats": ["wav", "mp3"],
    "supported_sample_rates": [16000, 22050, 24000, 44100],
    "default_format": "wav",
    "default_sample_rate": 24000,
    "max_upload_bytes": 10485760,
    "reference_audio_seconds": 5.0
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
        { "find": "\\n{3,}", "replace": "\n\n" }
      ]
    },
    "model_sort": ["fast", "clone", "emotion_v2"],
    "option_panel": {
      "emotion_v2": {
        "voice_type": "select",
        "speed_type": "slider",
        "pitch_type": "slider",
        "emotion_type": "select"
      },
      "clone": {
        "voice_type": "select",
        "speed_type": "slider",
        "voice_metadata_schema": [
          { "key": "name", "label": "Voice Name", "type": "text", "required": true },
          { "key": "gender", "label": "Gender", "type": "select", "options": ["Male", "Female", "Other"] }
        ]
      }
    }
  }
}
```

#### Capabilities: two levels, one rule

`capabilities` appears twice — once engine-wide, and once per mode. Resolution is a single
rule the platform applies everywhere:

> **A mode's own value wins if it states one; otherwise the engine-wide value applies;
> otherwise the platform default.**

Because of that, omitting a field is not the same as setting it to `false`:

| Mode states | Engine states | Result | Meaning |
|---|---|---|---|
| *(omitted)* | `true` | `true` | mode inherits |
| *(omitted)* | *(omitted)* | platform default | nothing stated anywhere |
| `false` | `true` | `false` | mode opts out of an engine-wide feature |
| `true` | `false` | `true` | mode opts in to something most modes lack |

Declare only what differs from the engine-wide set. The example above uses this to give a
single mode pitch and emotion while every other mode inherits `false`.

Platform defaults, applied when neither level states a value: `supports_preset_voices` and
`supports_streaming` and `supports_speed` are `true`; everything else is `false`.

#### `constraints.chunking`

Text longer than `max_text_length` is split by the platform before transmission, so this
governs data rather than presentation — which is why it sits under `constraints` and not
`ui_schema`.

* `max_chunk_size` — preferred chunk length. The platform **clamps this to
  `max_text_length`**, since a larger chunk would be rejected by the engine's own limit.
* `delimiters` — ordered list of cut points, most preferred first. The platform tries each
  in turn on any fragment still too long, then hard-splits whatever survives all of them.
  Any number of entries is allowed.

Patterns are applied with a split operation, so **zero-width lookbehinds are the expected
shape**: `(?<=[.!?]\s+)` marks a boundary after sentence-ending punctuation without
consuming characters. A pattern that matches content instead (e.g. `[^.!?]+[.!?]+`) still
works, but a split discards the matched delimiter itself, so lookbehind is preferred.

Invalid regex is ignored in favour of the platform default rather than failing the request.

#### `voice_metadata_schema` accepts any fields

Declare as many as the engine needs. `gender`, `region` and `style` have dedicated columns
because the platform filters and displays them; everything else is stored in a JSONB column
and returned unchanged. There is no fixed set and no upper bound — the form lays out to fit.

#### `audio_spec` reference-audio fields

`max_upload_bytes` and `reference_audio_seconds` describe what the engine accepts for voice
cloning, not what it produces.

* `max_upload_bytes` — largest reference clip the engine can process. The platform enforces
  whichever is stricter, this or the deployment's own `MAX_UPLOAD_SIZE_MB`, and rejects
  oversized uploads mid-transfer rather than buffering them first.
* `reference_audio_seconds` — how long a clip the engine wants. The waveform trimmer selects
  a window of exactly this length, and its labels read from the same number.

Both are optional; omit them and the platform assumes 10 MB and 5 seconds.

#### `ui_schema.option_panel` is presentation only

`voice_type`, `speed_type`, `pitch_type` and `emotion_type` choose **which widget draws a
control**, never whether the control exists — that is decided by capabilities alone. A
`pitch_type` on a mode whose resolved `supports_pitch` is `false` has no effect.



---

### 2.2 Voices Metadata API
* **Endpoint**: `GET /voices`
* **Description**: Returns the list of pre-loaded AI voices available in the Core Engine.

#### Response Example (`application/json`):
```json
[
  {
    "id": "james_doc",
    "name": "James Narrator",
    "gender": "male",
    "region": "North America",
    "style": "News",
    "language": "en-US"
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
  "text": "Hello world, this is the text segment to synthesize.",
  "voice_id": "james_doc",
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
