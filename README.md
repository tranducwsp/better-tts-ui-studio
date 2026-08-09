# 🎙️ AI Voice Studio - Universal Schema-Driven TTS Platform

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Svelte 5](https://img.shields.io/badge/Frontend-Svelte%205-orange.svg)
![Go Chi](https://img.shields.io/badge/Control%20Plane-Go%201.22-00ADD8.svg)
![Python FastAPI](https://img.shields.io/badge/Compute%20Engine-Python%203.10-green.svg)
![Docker Compose](https://img.shields.io/badge/Deployment-Docker%20Compose-2496ED.svg)

**AI Voice Studio** is a modern, high-performance, dynamic schema-driven Text-to-Speech (TTS) web application and control plane platform. Designed for AI Engineers, researchers, and enterprises, it allows integrating **ANY** custom AI Speech Synthesis engine (XTTS, GPT-SoVITS, VITS, Fish-Speech, Piper, Kokoro, etc.) without writing a single line of frontend or backend control code.

---

## 🌟 Key Features

- ⚡ **Dynamic Schema-Driven UI**: The entire user interface (options, sliders, tabs, notice banners, input rules) is generated dynamically at runtime from the AI engine's manifest (`GET /info`).
- 🪄 **Manifest Auto-Format Regex Rules**: Text hygiene and text normalization (gantry stripping, footnote removal, Vietnamese unicode fixes) are centralized in the Core TTS manifest.
- 🎙️ **Dynamic Voice Cloning Metadata**: Customize the voice creation form fields (Accent, Gender, Style, Age) directly via engine configuration schemas.
- 🔒 **Authentication & Multi-tenant Storage**: JWT HttpOnly session security (access + refresh token pair), mandatory authentication, per-user isolation of voices and history, and an Admin dashboard.
- ⚡ **High Concurrency Go Chi Control Plane**: Sub-millisecond latency router managing task queues, PostgreSQL database history, SSE progress streaming, and audio caching.
- 🎨 **Modern Futuristic UI**: Built with Svelte 5 (Runes), glassmorphism styling, real-time chunk audio playback, and intuitive search/replace text tools.

---

## 🏗️ Architecture Overview

```
 ┌────────────────────────────────────────────────────────┐
 │                   Frontend (Svelte 5)                  │
 │   - Dynamic UI Builder & Reactive Text Input Panel      │
 │   - Chunk Audio Player & Real-time Progress Streaming  │
 └───────────────────────────┬────────────────────────────┘
                             │ REST & SSE
 ┌───────────────────────────▼────────────────────────────┐
 │              Control Plane Gateway (Go Chi)            │
 │   - Authentication (JWT HttpOnly Cookies)              │
 │   - PostgreSQL Metadata & Job Task Queue Manager       │
 │   - User Voice Storage & Per-User Isolation              │
 └───────────────────────────┬────────────────────────────┘
                             │ gRPC / REST Core Protocol
 ┌───────────────────────────▼────────────────────────────┐
 │             Compute Engine Microservice (Python)       │
 │   - PyTorch / ONNX / CUDA Speech Synthesis Model       │
 │   - Dynamic Manifest (`GET /info`) & Metadata Schemas  │
 └────────────────────────────────────────────────────────┘
```

---

## 🚀 Quick Start with Docker Compose

Ensure you have **Docker** and **Docker Compose** installed.

```bash
# 1. Clone the repository
git clone https://github.com/your-username/better-tts-ui-studio.git
cd better-tts-ui-studio

# 2. Set the required secrets
cp .env.example .env
# Fill in at least SECRET_KEY, POSTGRES_PASSWORD and REDIS_PASSWORD —
# docker compose refuses to start while any of them is empty.
#   openssl rand -hex 32

# 3. Launch services using Docker Compose
docker compose up -d --build
```

Access the application in your browser:
- 🌐 **Web Studio UI**: [http://localhost:5173](http://localhost:5173)
- ⚙️ **Control Plane API**: [http://localhost:8000](http://localhost:8000)

Only those two ports are published. The compute engine, frontend builder, Postgres and Redis
stay on the internal compose network: the engine has no authentication of its own, so anything
that can reach it can spend your GPU and read other users' tasks. Reach them for debugging with
`docker compose exec` (e.g. `docker compose exec core-engine curl localhost:8001/health`).

---

## 📚 Documentation

Detailed documentation is available in the [`docs/`](./docs/) directory:

- 🗺️ [**Code Map**](./docs/CODE_MAP.md): Which file does what, and the order to read them in. Start here before reviewing.
- 📖 [**Frontend Overview (Giới thiệu chung FE)**](./docs/frontend_overview.md): High-level overview of the Svelte 5 Studio interface, features, and UI/UX design.
- 🛠️ [**Frontend Technical & Engineering Spec**](./docs/frontend_tech_and_engineering.md): Detailed technical spec on Svelte 5 Runes, Web Audio API binary handling, streaming, and build pipeline.
- 📖 [**AI Engineer Integration Guide**](./docs/ENGINEER_INTEGRATION_GUIDE.md): How to plug your custom AI Model into the system using Python schemas.
- 📜 [**Universal TTS Core Protocol Specification**](./docs/core_tts_protocol_spec.md): Complete REST API protocol reference.
- ⚙️ [**Configuration Reference**](./docs/CONFIGURATION.md): Which knob lives in the manifest, which in the environment, and why.
- 📦 [**Manifest Examples**](./docs/examples/): Real `/api/info` responses to adapt.
- 🕳️ [**Platform Gaps**](./docs/PLATFORM_GAPS.md): Manifest fields the platform does not honour yet, and what was already resolved.

---

## 🛠️ Tech Stack

- **Frontend**: Svelte 5 (Runes), Vite, FontAwesome 6, Vanilla CSS Glassmorphism.
- **Control Plane**: Go 1.22, Chi Router, Sonic JSON, PostgreSQL 16, Redis.
- **Compute Engine**: Python 3.10, PyTorch, FastAPI / Uvicorn.

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
