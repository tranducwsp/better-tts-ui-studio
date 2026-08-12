# 🌐 Gateway API Reference & Complete Engine Manifest Writing Guide

This document provides the complete list of all **Control Plane Gateway** API Endpoints and the **Full Field-by-Field Specification** of the **Engine Manifest (`GET /info`)**.

---

## 📡 1. Detailed Gateway Endpoint List

All APIs working with application data are under the `/api` prefix.

### 1.1. System Health Check Endpoints (Public / System)

| Method | Endpoint | Description | Protection / Rate Limit |
| :--- | :--- | :--- | :--- |
| `GET` | `/health` | Liveness probe to check if the application is running | Public |
| `GET` | `/ready` | Readiness probe to check DB & Engine connection | Public |
| `GET` | `/swagger` | Serve Swagger UI API documentation interface | Public (if `ENABLE_SWAGGER=true`) |
| `GET` | `/debug/pprof/*` | Go Profiler to see RAM/Goroutine memory status | Public (if `ENABLE_PPROF=true`) |

---

### 1.2. Authentication & Engine Information Endpoints (Auth & Public API)

| Method | Endpoint | Request Payload / Query | Response Content | Description |
| :--- | :--- | :--- | :--- | :--- |
| `GET` | `/api/info` | - | `JSON Manifest` | Return Engine, Models, Capabilities, UI Schema & Voice Cloning information |
| `POST` | `/api/register` | `{ "username", "password", "email" }` | `{ "message", "user" }` | Register a new account (Rate Limited) |
| `POST` | `/api/login` | `{ "username", "password" }` | `{ "message", "user" }` + Set HttpOnly Cookie | Login to the system, issue Access Token & Refresh Token Cookies |
| `POST` | `/api/auth/refresh` | (Read Refresh Token from Cookie) | `{ "message" }` + Set new Access Token Cookie | Issue a new Access Token when expired |
| `POST` | `/api/auth/logout` | (Read Session Cookie) | `{ "message" }` + Clear Cookies | Logout and revoke the server session |

---

### 1.3. User Endpoints for Synthesis & Voice Management (Protected User APIs)

> **Requirement**: Valid Access Token Cookie of an activated account (`RequireActiveUser`).

| Method | Endpoint | Payload / Parameters | Response Content | Description |
| :--- | :--- | :--- | :--- | :--- |
| `GET` | `/api/me` | - | `{ "user": { "id", "username", "role", "is_active" } }` | Get personal information of the currently logged-in account |
| `GET` | `/api/voices/{model_id}` | `model_id` in path | `{ "voices": [...] }` | Get the Model's voice list (including system voices & personal clone voices) |
| `POST` | `/api/jobs/init` | `{ "model_id", "total_chunks", "text_preview" }` | `{ "job_id", "status" }` | Initialize a multi-segment text synthesis Job |
| `POST` | `/api/synthesize/{model_id}` | `{ "job_id", "chunk_index", "text", "voice_id", "params": {...} }` | `{ "task_id", "status": "pending" }` (HTTP 202) | Send a synthesis request for 1 text chunk |
| `GET` | `/api/history` | `?page=1&limit=20` | `{ "jobs": [...], "total" }` | View personal synthesis history |
| `GET` | `/api/history/{job_id}` | `job_id` in path | `{ "job": {...}, "chunks": [...] }` | View detailed information of a created Job |
| `POST` | `/api/clone/upload` | Multipart Form: `file` (WAV/MP3), `name`, `gender`, `accent`, `age` | `{ "voice": { "id", "name", "file_path" } }` | Upload a sample voice file to create a personal Clone voice |
| `POST` | `/api/clone/upload-temp` | Multipart Form: `file` | `{ "temp_id", "temp_path" }` | Upload temporary audio to preview before creating a voice |
| `GET` | `/api/clone/voices` | - | `{ "voices": [...] }` | List of personal Clone voices uploaded by the user |
| `DELETE` | `/api/clone/voices/{clone_id}`| `clone_id` in path | `{ "message": "Voice deleted" }` | Delete a personal Clone voice |
| `GET` | `/api/tasks/{task_id}` | `task_id` in path | `{ "task_id", "status", "progress", "error" }` | Look up the processing status of a Task Chunk |
| `POST` | `/api/tasks/{task_id}/cancel`| `task_id` in path | `{ "message": "Task cancelled" }` | Cancel a Task Chunk waiting in the queue |
| `GET` | `/api/tasks/{task_id}/audio` | `task_id` in path | Audio Binary Stream (`audio/wav`) | Download the resulting audio file after the Task completes |
| `GET` | `/api/stream/tasks/{task_id}`| `task_id` in path | Server-Sent Events (SSE Stream) | Open an SSE connection to receive real-time Task Chunk progress |
| `POST` | `/api/extract-text` | Multipart Form: `file` (PDF/DOCX/TXT) | `{ "text": "..." }` | Extract raw text from a document file (Rate & Concurrency Limited) |

---

### 1.4. System Administration Endpoints (Admin APIs)

> **Requirement**: Access Token Cookie of an account with `admin` role (`RequireAdmin`).

| Method | Endpoint | Payload / Parameters | Description |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/internal/engine/reload` | - | Force the Gateway to reload the Manifest from the AI Engine and fire a Webhook telling the Frontend Builder to prerender the UI bundle again |
| `GET` | `/api/admin/users` | `?page=1&limit=20` | View the full list of users in the system |
| `POST` | `/api/admin/users/{user_id}/approve` | `user_id` in path | Approve (activate) an account for a newly registered user |
| `GET` | `/api/admin/users/{user_id}/history` | `user_id` in path | Check the full synthesis history of any user |

---

## 📄 2. DETAILED MANIFEST SPECIFICATION (`GET /info`) - FIELD-BY-FIELD STRUCTURE

The Engine Manifest is a complex JSON object containing 8 main blocks. Below is the detailed specification of each block, each field, and the list of allowed options:

```
UniversalManifest (Root)
 ├── engine_id, engine_name, version, provider
 ├── supported_modes [ EngineModeSpec ]
 ├── capabilities (EngineCapabilities)
 ├── constraints (EngineConstraints)
 ├── audio_spec (AudioSpec)
 └── ui_schema (UISchemaSpec)
```

---

### 2.1. Root Properties Block

| Field | Data Type | Required | Default Value | Description & Options |
| :--- | :--- | :--- | :--- | :--- |
| `engine_id` | `string` | **Yes** | - | Unique identifier of the Engine (e.g., `"vits-vietnamese"`, `"xtts-v2"`, `"piper-tts"`) |
| `engine_name` | `string` | **Yes** | - | Display name on the interface (e.g., `"VITS Neural Studio"`, `"XTTS Voice Synthesizer"`) |
| `version` | `string` | **Yes** | `"1.0.0"` | Engine version (e.g., `"1.0.0"`, `"2.1-beta"`) |
| `provider` | `string` | No | `""` | Name of the organization or development team (e.g., `"DeepMind AI"`, `"Open Source Community"`) |

---

### 2.2. `supported_modes` Block (List of Models / Processing Modes)

The `supported_modes` array contains `EngineModeSpec` objects defining each processing mode Tab on the interface (e.g., Super-fast Mode, Standard Mode, Clone Mode).

| Field | Data Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `id` | `string` | **Yes** | Mode identifier (e.g., `"fast"`, `"standard"`, `"clone"`, `"express"`) |
| `name` | `string` | **Yes** | Display name of the Mode Tab (e.g., `"Super Fast"`, `"Neural Standard"`, `"Clone Voice"`) |
| `description` | `string` | No | Short description of the Mode's characteristics |
| `capabilities` | `object` | No | Override Capabilities specifically for this Mode (see Block 2.3) |
| `audio_spec` | `object` | No | Override AudioSpec specifically for this Mode (see Block 2.5) |

---

### 2.3. `capabilities` Block (Supported Features - Capabilities)

This block can be declared at the **Engine-wide level** or **overridden at the Mode level**. All `boolean` fields accept 3 values: `true` (On), `false` (Off), or `null/omitted` (Inherit).

| Field | Data Type | Platform Default | Description & Interface Impact |
| :--- | :--- | :--- | :--- |
| `supports_preset_voices` | `boolean` | `true` | Enable/Disable the available voice list (System Preset Voices). If `true`, display a Voice Selection Dropdown |
| `supports_cloning` | `boolean` | `false` | Enable/Disable the feature to upload a sample audio file to Clone a voice immediately. If `true`, display Dropzone and Waveform Trimmer |
| `supports_voice_saving` | `boolean` | `false` | Enable/Disable the feature to save a Clone voice to the personal library long-term. If `true`, display the `+ Save New Voice` button |
| `supports_streaming` | `boolean` | `true` | Enable/Disable the text chunking mechanism and real-time playback via SSE Stream |
| `supports_speed` | `boolean` | `true` | Enable/Disable the reading speed control (Speed slider/number) |
| `supports_pitch` | `boolean` | `false` | Enable/Disable the voice pitch control (Pitch slider/number) |
| `supports_emotion` | `boolean` | `false` | Enable/Disable the voice emotion selection (Emotion select/radio) |
| `supports_ssml` | `boolean` | `false` | Enable/Disable the SSML tag input and parsing mode |

---

### 2.4. `constraints` Block (Technical Constraints & Limits)

Defines the parameter boundaries for the Frontend to generate sliders and validate data before sending requests.

```json
"constraints": {
  "max_text_length": 3000,
  "speed_range": { "min": 0.5, "max": 2.0, "default": 1.0, "step": 0.1 },
  "pitch_range": { "min": -10.0, "max": 10.0, "default": 0.0, "step": 0.5 },
  "supported_emotions": ["happy", "sad", "angry", "fearful", "neutral"],
  "chunking": {
    "max_chunk_size": 1000,
    "delimiters": ["\n\n", "\n", ". ", "; ", ", "]
  }
}
```

| Detailed Field | Data Type | Default | Description & Options |
| :--- | :--- | :--- | :--- |
| `max_text_length` | `integer` | `3000` | Maximum characters for 1 synthesis request. If text is longer, the system automatically splits into chunks |
| `speed_range.min` | `float` | `0.5` | Minimum speed value |
| `speed_range.max` | `float` | `2.0` | Maximum speed value |
| `speed_range.default` | `float` | `1.0` | Default speed value |
| `speed_range.step` | `float` | `0.1` | Step increment when sliding the control |
| `pitch_range.min` | `float` | `-10.0` | Minimum pitch value |
| `pitch_range.max` | `float` | `10.0` | Maximum pitch value |
| `pitch_range.default` | `float` | `0.0` | Default pitch value |
| `pitch_range.step` | `float` | `0.5` | Pitch step increment |
| `supported_emotions` | `array[string]`| `[]` | Array of emotion keywords supported by the model (e.g., `["neutral", "happy", "sad"]`) |
| `chunking.max_chunk_size` | `integer` | `1000` | Maximum characters per small text Chunk |
| `chunking.delimiters` | `array[string]`| `["\n\n", "\n", ". "]`| Priority cut points for text segmentation from high to low to avoid breaking sentences |

---

### 2.5. `audio_spec` Block (Audio Format Parameters)

Specifies the output file standards and input reference audio file standards.

| Field | Data Type | Default | Description & Valid Values |
| :--- | :--- | :--- | :--- |
| `supported_formats` | `array[string]` | `["wav"]` | Array of supported output formats: `"wav"`, `"mp3"`, `"flac"`, `"ogg"`, `"aac"` |
| `supported_sample_rates` | `array[int]` | `[24000]` | Array of supported output sample rates: `16000`, `22050`, `24000`, `44100`, `48000` |
| `default_format` | `string` | `"wav"` | Default audio format |
| `default_sample_rate` | `integer` | `24000` | Default sample rate (Hz) |
| `reference_audio_formats` | `array[string]` | `["wav"]` | Array of accepted reference audio file formats for upload: `"wav"`, `"mp3"`, `"ogg"`, `"flac"`, `"m4a"` |
| `reference_audio_seconds` | `float` | `5.0` | Recommended seconds for a standard reference audio clip |
| `max_upload_bytes` | `int64` | `104857600` (100MB) | Maximum raw file size allowed to be dragged and dropped into the browser |
| `max_reference_bytes` | `int64` | `10485760` (10MB) | Maximum size of the trimmed audio clip sent to the Engine |

---

### 2.6. `ui_schema` Block (Detailed Dynamic Interface Specification)

This is the block that determines the visual presentation of control components.

```json
"ui_schema": {
  "ui_mode": "beauty",
  "model_sort": ["fast", "standard", "clone"],
  "input_panel": {
    "file_serve": true,
    "closeable": false,
    "find_mode": "expert",
    "replace_tool": true,
    "enable_chunk_box": true,
    "auto_format": [
      { "find": "\\([^)]*\\)", "replace": "" }
    ]
  },
  "option_panel": {
    "standard": {
      "notice_banner": {
        "level": "info",
        "message": "Standard model uses 24kHz Neural network."
      },
      "voice_type": "select",
      "speed_type": "slider",
      "pitch_type": "slider",
      "emotion_type": "select",
      "preset_voices": [
        {
          "id": "vi_female_1",
          "name": "Standard Hanoi Female",
          "metadata": {"gender": "female", "accent": "Northern", "style": "Expressive"},
          "sample_url": "/samples/vi_female_1.wav"
        }
      ],
      "voice_metadata_schema": [
        {
          "key": "accent",
          "label": "Regional Accent",
          "type": "select",
          "required": true,
          "options": ["Northern", "Southern", "Central"]
        }
      ]
    }
  }
}
```

#### 2.6.1. Detailed Fields in `ui_schema`:

1. **`ui_mode`** (`string`):
   - `"beauty"` *(Default)*: Enable glassmorphism effects, modern colors, GPU blur effects, rich FontAwesome icons.
   - `"fast"`: Disable all GPU filters, remove heavy icons and fonts, optimize for ultra-lightweight 0ms FCP websites for weak devices or slow networks.

2. **`model_sort`** (`array[string]`):
   - Array containing the list of `mode_id` to determine the **display order of Tabs on the Menu bar**. Example: `["fast", "standard", "clone"]`.

3. **`input_panel`** (Text input panel configuration):
   - `file_serve` (`boolean`, default `true`): Allow/Disallow uploading document files (PDF, DOCX, TXT) to read text.
   - `closeable` (`boolean`, default `false`): Allow closing the text input panel.
   - `find_mode` (`string`): `"expert"` (full-featured advanced search) or `"express"` (simple quick search).
   - `replace_tool` (`boolean`, default `true`): Enable/Disable the Search & Replace tool.
   - `enable_chunk_box` (`boolean`, default `true`): Enable/Disable the chunk preview box display.
   - `auto_format` (`array[AutoFormatRule]`): List of automatic text normalization Regex rules `[{ "find": "regex_pattern", "replace": "replacement_string" }]`.

4. **`option_panel`** (Map of options specific to each `mode_id`):
   - `notice_banner`: Visual banner at the top of the control panel:
     - `level` (Enum): `"info"` (Blue), `"success"` (Green), `"warning"` (Yellow), `"danger"` / `"error"` (Red).
     - `message`: Notification message string.
   - `voice_type` (Enum): `"select"` (Render as Dropdown) or `"radio"` (Render as quick-select Radio buttons).
   - `speed_type` (Enum): `"slider"` (Slider) or `"number"` (Direct number input box).
   - `pitch_type` (Enum): `"slider"` (Slider) or `"number"` (Direct number input box).
   - `emotion_type` (Enum): `"select"` (Dropdown emotion selection) or `"radio"` (Radio emotion selection).
   - `preset_voices` (Array): Declare the list of fixed Model voices directly in the Manifest (see Block 2.6.2).
   - `voice_metadata_schema` (Array): Declare the attribute registration form when creating a new Clone voice (see Block 2.6.3).

---

#### 2.6.2. Fixed Voice Structure Details (`preset_voices`)

Used to declare the list of standard Model voices directly in the Manifest (great for SEO and static pages):

| Field | Data Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `id` | `string` | **Yes** | Unique ID of the voice (e.g., `"vi_female_hn"`) |
| `name` | `string` | **Yes** | Display name of the voice (e.g., `"Hanoi Female - Thanh Ngan"`) |
| `metadata` | `object` | No | Dynamic key-value metadata (e.g., `{"accent": "Northern", "style": "Expressive"}`). Displayed as badges in the Voice Select dropdown. |
| `sample_url` | `string` | No | Sample audio file URL for preview (e.g., `"/static/samples/hn_female.wav"`) |

---

#### 2.6.3. Clone Attribute Form Structure Details (`voice_metadata_schema`)

Used to automatically generate data input fields (such as Region, Gender, Style) in the Create New Clone Voice Modal [`CreateVoiceModal`](file:///home/amoratran/server/better-tts-ui-studio/frontend/src/lib/components/CreateVoiceModal.svelte):

| Field | Data Type | Required | Description & Options |
| :--- | :--- | :--- | :--- |
| `key` | `string` | **Yes** | Attribute name in JSON (e.g., `"accent"`, `"gender"`, `"age_group"`) |
| `label` | `string` | **Yes** | Display label on the Form (e.g., `"Voice Regional Accent"`) |
| `type` | `string` | **Yes** | Input type: `"text"` (Text input box) or `"select"` (Dropdown selection box) |
| `required` | `boolean` | No | `true` if the user must fill/select |
| `placeholder` | `string` | No | Hint string in the input box |
| `options` | `array[string]`| No | List of options for `"select"` type (e.g., `["Northern", "Southern", "Central"]`) |

---

## 💻 3. COMPLETE STANDARD MANIFEST EXAMPLE (COMPLETE JSON MANIFEST EXAMPLE)

AI Engineers can copy the complete Manifest template below to apply directly to the `GET /info` endpoint on their Python service:

```json
{
  "engine_id": "neural-tts-vietnamese",
  "engine_name": "Vietnamese Neural TTS Studio",
  "version": "2.0.0",
  "provider": "AI Speech Lab",
  "supported_modes": [
    {
      "id": "fast",
      "name": "Super Fast (Fast Mode)",
      "description": "High-speed speech synthesis under 200ms",
      "capabilities": {
        "supports_preset_voices": true,
        "supports_cloning": false,
        "supports_speed": true,
        "supports_pitch": false
      }
    },
    {
      "id": "standard",
      "name": "Standard (Neural High-Res)",
      "description": "Natural expressive voice 24kHz",
      "capabilities": {
        "supports_preset_voices": true,
        "supports_cloning": true,
        "supports_voice_saving": true,
        "supports_speed": true,
        "supports_pitch": true,
        "supports_emotion": true
      }
    }
  ],
  "capabilities": {
    "supports_preset_voices": true,
    "supports_cloning": true,
    "supports_voice_saving": true,
    "supports_streaming": true,
    "supports_speed": true,
    "supports_pitch": true,
    "supports_emotion": true,
    "supports_ssml": false
  },
  "constraints": {
    "max_text_length": 5000,
    "speed_range": { "min": 0.5, "max": 2.0, "default": 1.0, "step": 0.1 },
    "pitch_range": { "min": -10.0, "max": 10.0, "default": 0.0, "step": 0.5 },
    "supported_emotions": ["neutral", "happy", "sad", "angry", "news"],
    "chunking": {
      "max_chunk_size": 1000,
      "delimiters": ["\n\n", "\n", ". ", "; "]
    }
  },
  "audio_spec": {
    "supported_formats": ["wav", "mp3", "flac"],
    "supported_sample_rates": [16000, 24000, 44100],
    "default_format": "wav",
    "default_sample_rate": 24000,
    "reference_audio_formats": ["wav", "mp3", "flac"],
    "reference_audio_seconds": 5.0,
    "max_upload_bytes": 104857600,
    "max_reference_bytes": 10485760
  },
  "ui_schema": {
    "ui_mode": "beauty",
    "model_sort": ["fast", "standard"],
    "input_panel": {
      "file_serve": true,
      "closeable": false,
      "find_mode": "expert",
      "replace_tool": true,
      "enable_chunk_box": true,
      "auto_format": [
        { "find": "\\([^)]*\\)", "replace": "" },
        { "find": "\\.{2,}", "replace": "." }
      ]
    },
    "option_panel": {
      "standard": {
        "notice_banner": {
          "level": "info",
          "message": "Model supports full emotion customization and personal voice cloning."
        },
        "voice_type": "select",
        "speed_type": "slider",
        "pitch_type": "slider",
        "emotion_type": "select",
        "preset_voices": [
          {
            "id": "vi_female_hn",
            "name": "Hanoi Female - Thanh Ngan",
            "metadata": {"gender": "female", "accent": "Northern", "style": "Expressive"},
            "sample_url": "/samples/hn_female.wav"
          },
          {
            "id": "vi_male_sg",
            "name": "Saigon Male - Minh Triet",
            "metadata": {"gender": "male", "accent": "Southern", "style": "Warm"},
            "sample_url": "/samples/sg_male.wav"
          }
        ],
        "voice_metadata_schema": [
          {
            "key": "accent",
            "label": "Voice Regional Accent",
            "type": "select",
            "required": true,
            "options": ["Northern", "Southern", "Central"]
          },
          {
            "key": "gender",
            "label": "Gender",
            "type": "select",
            "required": true,
            "options": ["Female", "Male"]
          },
          {
            "key": "style",
            "label": "Reading Style",
            "type": "text",
            "required": false,
            "placeholder": "Example: News, Storytelling, Advertising..."
          }
        ]
      }
    }
  }
}
```