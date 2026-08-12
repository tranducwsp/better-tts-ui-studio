# 🎙️ Better TTS UI Studio - Universal Dynamic Schema-Driven TTS Platform

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Svelte 5](https://img.shields.io/badge/Frontend-Svelte%205-orange.svg)
![Go Chi](https://img.shields.io/badge/Control%20Plane-Go%201.22-00ADD8.svg)
![Python FastAPI](https://img.shields.io/badge/Compute%20Engine-Python%203.10+-green.svg)
![Docker Compose](https://img.shields.io/badge/Deployment-Docker%20Compose-2496ED.svg)

**Better TTS UI Studio** is a modern, high-performance Web UI & Control Plane platform designed for **AI Engineers** and Text-to-Speech (TTS) model development teams.

The platform is built on a **Dynamic Schema-Driven (Manifest-Driven)** architecture: It allows integrating **ANY** AI Speech Synthesis model (XTTS, GPT-SoVITS, VITS, Fish-Speech, Piper, Kokoro, F5-TTS, VieNeu, etc.) into a professional interface and management system **without writing or modifying any lines of Go Gateway or Svelte Frontend source code!**

---

## 🌟 Why Choose Better TTS UI Studio?

Designed for AI Engineers who want to take their models from notebooks/local scripts to real products:

- ⚡ **100% Dynamic Interface (Schema-Driven UI)**: The entire parameter control panel (sliders, dropdowns, switches, notice banners, input rules) is automatically generated at runtime through the **Manifest file (`GET /info`)** declared by your AI Model.
- 🪄 **Automatic Text Cleaning (Regex Rule Engine)**: Declare text normalization rules (removing special characters, fixing Vietnamese Unicode errors, removing gantry text, etc.) directly in the Manifest so the Gateway and Frontend handle them before sending to the model.
- 🎙️ **Flexible Voice Cloning**: Define voice sample attribute fields (Accent, Gender, Style, Age, etc.) entirely through JSON Schema.
- 🔒 **Full Enterprise Features**: Built-in Authentication (JWT HttpOnly Cookies), User Authorization (User / Admin), Synthesis History Management, Multi-platform Storage (Local Disk / AWS S3 / MinIO).
- ⚡ **High Concurrency Queue Processing**: Go Chi Backend combined with Redis Task Queue, enabling real-time progress streaming via SSE (Server-Sent Events) and per-chunk audio playback on the interface.

---

## 🏗️ Platform Architecture Overview

```
 ┌────────────────────────────────────────────────────────┐
 │                   Frontend (Svelte 5)                  │
 │   - Auto-generate UI from Manifest                      │
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
 │        Your AI Model Service (Python Core TTS)          │
 │   - PyTorch / ONNX / CUDA Speech Model                 │
 │   - Provide Manifest via `GET /info`                   │
 └────────────────────────────────────────────────────────┘
```

---

## 🚀 Quick Start Guide for AI Engineers Deploying Models

### Step 1: Prepare Your AI Model Microservice

The platform communicates with your AI model through a standard HTTP REST (or gRPC) Service. We have provided a complete example source code in the [`core-tts-example/`](./core-tts-example) directory.

Your AI service only needs to implement 3 main endpoints:
1. `GET /info` (Required): Returns a JSON Manifest defining model information, UI parameter panel, regex rules, and Voice Cloning features.
2. `GET /voices` (Required): Returns the list of available voices for the model.
3. `POST /tts` (Or gRPC `Synthesize`): Receives text + parameter configuration and returns an audio file (WAV/MP3/FLAC/OGG).
4. `POST /clone` (Optional): Receives a sample audio file and metadata to register a new cloned voice.

### Step 2: Write the Manifest (`GET /info`) to Build the Interface

This is the key point: **The user interface will reflect exactly the JSON structure you return from `GET /info`**.

Basic Manifest structure example:

```json
{
  "engine": {
    "id": "my-custom-tts",
    "name": "My Custom Neural TTS",
    "version": "1.0.0",
    "description": "High-speed AI speech synthesis model"
  },
  "models": [
    {
      "id": "my-model-v1",
      "name": "Standard Vietnamese Model",
      "sample_rate": 24000,
      "supported_formats": ["wav", "mp3"]
    }
  ],
  "ui_schema": {
    "components": [
      {
        "id": "temperature",
        "label": "Creativity (Temperature)",
        "type": "slider",
        "default": 0.7,
        "min": 0.1,
        "max": 1.0,
        "step": 0.05
      },
      {
        "id": "speed",
        "label": "Speech Speed",
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

👉 See detailed Manifest structure and all supported components at [docs/GATEWAY.md](./docs/GATEWAY.md).

### Step 3: Launch the Platform with Docker Compose

1. **Clone the repository**:
   ```bash
   git clone https://github.com/your-username/better-tts-ui-studio.git
   cd better-tts-ui-studio
   ```

2. **Configure the `.env` file**:
   ```bash
   cp .env.example .env
   ```
   Open the `.env` file and fill in the security keys (you can generate them with `openssl rand -hex 32`):
   - `SECRET_KEY`: JWT signing key.
   - `POSTGRES_PASSWORD`: Database password.
   - `REDIS_PASSWORD`: Redis cache/queue password.
   - `CORE_ENGINE_URL`: URL to your AI Model service (default in docker compose is `http://core-engine:8001`).

3. **Launch the system**:
   ```bash
   docker compose up -d --build
   ```

4. **Access the application**:
   - 🌐 **Web Studio UI**: [http://localhost:5173](http://localhost:5173)
   - ⚙️ **Control Plane API**: [http://localhost:8000](http://localhost:8000)

When you change the Manifest or upgrade the AI Model, simply call the re-sync endpoint:
```bash
curl -X POST http://localhost:8000/api/internal/engine/reload
```
The platform will automatically update the latest UI for all users immediately!

---

## 📚 Complete Technical Documentation System

To learn more about each component in the system, refer to the in-depth documentation in the `docs/` directory:

| Document File | Main Content |
| :--- | :--- |
| 🗺️ [**docs/SOURCE.md**](./docs/SOURCE.md) | Explanation of the entire project source code structure (`backend`, `frontend`, `core-tts-example`, `k8s`). |
| 🏗️ [**docs/ARCHITECTURE.md**](./docs/ARCHITECTURE.md) | Production deployment system architecture, data flow, Task Workers, and Cron/Sweeper jobs. |
| 🌐 [**docs/GATEWAY.md**](./docs/GATEWAY.md) | Detailed Gateway REST API Endpoints and complete guide to writing the Manifest (`GET /info`). |
| ⚙️ [**docs/CONFIG.md**](./docs/CONFIG.md) | Detailed guide to configuring Environment Variables for the Backend. |
| 🎨 [**docs/FRONTEND.md**](./docs/FRONTEND.md) | Frontend Technical Specification (Svelte 5 Runes, Web Audio API binary player, SSE streaming, Dynamic UI rendering). |
| 🔧 [**docs/BACKEND.md**](./docs/BACKEND.md) | Backend Technical Specification (Go Chi, Redis Stream Task Queue, Storage Abstraction, JWT Auth & Database Connection Pool). |

---

## 🛠️ Technologies Used

- **Frontend**: Svelte 5 (Runes state management), Vite, FontAwesome 6, Vanilla CSS Glassmorphism.
- **Control Plane Gateway**: Go 1.22, Chi Router, Sonic JSON, PostgreSQL 16 (pgxpool), Redis Stream & Cache.
- **Compute Engine**: Python 3.10+, PyTorch / ONNX / CUDA, FastAPI / Uvicorn / gRPC.
- **Orchestration**: Docker, Docker Compose, Kubernetes (K3s manifests).

---

## 📄 License

The project is released under the [MIT License](LICENSE).