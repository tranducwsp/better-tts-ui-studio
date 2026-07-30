# 🧪 Core TTS Mock & Test Microservice (`core-tts-test`)

`core-tts-test` is a lightweight, zero-ML-dependency mock microservice designed for instant testing, frontend UI development, and stress testing without loading PyTorch weights or calling external edge-tts services.

---

## 🌟 Key Features

- **⚡ Zero Heavy AI Dependencies**: Uses Python standard libraries (`wave`, `math`, `struct`) to synthesize a clean PCM 16-bit 24kHz WAV audio buffer in-memory.
- **⏱️ Simulated Delay & Latency**: Configure response delay using `MOCK_DELAY_SEC` (default `0.2s`).
- **⏳ Simulated Timeout Testing**: Send text containing `"timeout"` or `"simulate_timeout"` to force a 35-second delay to test gateway/frontend timeout handling.
- **💥 Simulated Error Testing**: Send text containing `"error"` or `"simulate_error"` to trigger an HTTP 500 error for testing error handling.
- **📜 100% Core TTS Protocol Compliant**: Exposes `GET /info`, `GET /voices`, `GET /health`, `POST /synthesize`, `GET /tasks/{task_id}`, `POST /voices/clone`.

---

## 🚀 How to Run

### Option 1: Using shell script
```bash
cd core-tts-test
./run_mock.sh
```

### Option 2: Using Docker Compose
In `docker-compose.yml`, point `core-tts` build context to `./core-tts-test`:
```yaml
core-tts:
  build:
    context: ./core-tts-test
```
Or run directly:
```bash
docker build -t core-tts-test ./core-tts-test
docker run -p 8001:8001 core-tts-test
```

---

## 🧪 Testing Triggers

| Request Text Input | Behavior |
| :--- | :--- |
| Any normal text (e.g. `"Hello World"`) | Instant mock WAV audio response with `0.2s` delay |
| Contains `"timeout"` or `"simulate_timeout"` | Sleeps for 35s to test gateway/frontend timeout handling |
| Contains `"error"` or `"simulate_error"` | Returns 500 Internal Server Error |
