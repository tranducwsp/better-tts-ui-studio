import os
import time
import uuid
import asyncio
from typing import Dict, Any
from fastapi import FastAPI, HTTPException, Response, UploadFile, File, Form
from fastapi.middleware.cors import CORSMiddleware

from schemas import SynthesizeRequest, TaskStatusResponse, UniversalManifest, VoiceInfo

# Pre-load static audio files into RAM once at startup for zero CPU overhead
STATIC_DIR = os.path.join(os.path.dirname(__file__), "static")
WAV_PATH = os.path.join(STATIC_DIR, "sample.wav")
MP3_PATH = os.path.join(STATIC_DIR, "sample.mp3")

if os.path.exists(WAV_PATH):
    with open(WAV_PATH, "rb") as f:
        STATIC_WAV_BYTES = f.read()
else:
    STATIC_WAV_BYTES = b"RIFF....WAVEfmt ....data...." # Fallback dummy header

if os.path.exists(MP3_PATH):
    with open(MP3_PATH, "rb") as f:
        STATIC_MP3_BYTES = f.read()
else:
    STATIC_MP3_BYTES = STATIC_WAV_BYTES

app = FastAPI(
    title="Core TTS High-Throughput Mock & Load Test Microservice",
    description="Ultra-fast mock server using pre-loaded static in-memory audio files for zero CPU overhead stress testing.",
    version="1.0.0-mock"
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Configuration from Environment Variables
DEFAULT_MOCK_DELAY = float(os.getenv("MOCK_DELAY_SEC", "0.0")) # Default 0.0s for max stress testing
PORT = int(os.getenv("CORE_PORT", "8001"))
HOST = os.getenv("CORE_HOST", "0.0.0.0")

# In-memory Tasks and Cloned Voices DB
tasks_db: Dict[str, Dict[str, Any]] = {}
cloned_voices_db: Dict[str, Dict[str, Any]] = {}

MOCK_VOICES = [
    {"id": "Mock Voice A", "name": "Mock Voice A", "descriptions": ["Female", "Northern", "Expressive"]},
    {"id": "Mock Voice B", "name": "Mock Voice B", "descriptions": ["Male", "Southern", "News"]},
    {"id": "Mock Voice C", "name": "Mock Voice C", "descriptions": ["Male", "Central", "Natural"]}
]

@app.get("/", tags=["Info"])
def root():
    return {
        "service": "Core TTS Zero-CPU Static Load Test Server",
        "status": "running",
        "mock_delay_sec": DEFAULT_MOCK_DELAY,
        "static_wav_bytes": len(STATIC_WAV_BYTES),
        "docs": "/docs"
    }

@app.get("/info", response_model=UniversalManifest, tags=["Manifest"])
def get_info():
    """Returns dynamic Universal Manifest spec."""
    return UniversalManifest()

@app.get("/voices", response_model=list[VoiceInfo], tags=["Voices"])
def get_voices():
    """Returns available mock voices."""
    return [VoiceInfo(**v) for v in MOCK_VOICES]

@app.get("/health", tags=["Health"])
def health_check():
    """Mock GPU liveness check."""
    return {
        "status": "healthy",
        "gpu_available": True,
        "vram_used_mb": 512,
        "vram_total_mb": 16384,
        "active_jobs": 0
    }

@app.post("/synthesize", tags=["Synthesis"])
async def synthesize(req: SynthesizeRequest):
    """
    Returns pre-loaded static WAV bytes instantly for maximum throughput load testing.
    - Text contains 'timeout' or 'simulate_timeout': Delays 35s to test timeout handling.
    - Text contains 'error' or 'simulate_error': Returns 500 error.
    """
    text_lower = req.text.strip().lower()
    if not text_lower:
        raise HTTPException(status_code=400, detail="Empty text input")

    task_id = req.task_id or str(uuid.uuid4())

    # 1. Simulated timeout trigger (Sleeps 65s > 60s backend client timeout)
    if "timeout" in text_lower or "simulate_timeout" in text_lower:
        await asyncio.sleep(65.0)
        raise HTTPException(status_code=504, detail="Simulated Core TTS Timeout")

    # 2. Simulated slow delay trigger (Sleeps 5s)
    if "slow" in text_lower or "delay" in text_lower:
        await asyncio.sleep(5.0)

    # 3. Simulated error trigger
    if "error" in text_lower or "simulate_error" in text_lower:
        raise HTTPException(status_code=500, detail="Simulated Core TTS Error")

    # 3. Simulated delay (if configured via env MOCK_DELAY_SEC)
    if DEFAULT_MOCK_DELAY > 0:
        await asyncio.sleep(DEFAULT_MOCK_DELAY)

    # 4. Instant response from static RAM buffer
    audio_bytes = STATIC_MP3_BYTES if req.output_format == "mp3" else STATIC_WAV_BYTES

    tasks_db[task_id] = {
        "status": "done",
        "progress": 100,
        "audio": audio_bytes,
        "created_at": time.time()
    }

    media_type = "audio/mpeg" if req.output_format == "mp3" else "audio/wav"

    return Response(
        content=audio_bytes,
        media_type=media_type,
        headers={"X-Task-ID": task_id}
    )

@app.get("/tasks/{task_id}", response_model=TaskStatusResponse, tags=["Tasks"])
def get_task_status(task_id: str):
    """Returns task status."""
    return TaskStatusResponse(
        task_id=task_id,
        status="done",
        progress=100,
        audio_url=f"/tasks/{task_id}/audio"
    )

@app.get("/tasks/{task_id}/audio", tags=["Tasks"])
def get_task_audio(task_id: str, format: str = "wav"):
    """Returns pre-loaded static audio bytes instantly from RAM."""
    audio_bytes = STATIC_MP3_BYTES if format.lower() == "mp3" else STATIC_WAV_BYTES
    media_type = "audio/mpeg" if format.lower() == "mp3" else "audio/wav"
    return Response(content=audio_bytes, media_type=media_type)

@app.delete("/tasks/{task_id}", tags=["Tasks"])
def cancel_task(task_id: str):
    """Simulates task cancellation."""
    return {"message": "Cancellation requested", "task_id": task_id}

@app.post("/voices/clone", tags=["Voice Cloning"])
async def clone_voice(file: UploadFile = File(...), name: str = Form(...)):
    """Simulates instant voice cloning."""
    clone_id = f"clone_mock_{uuid.uuid4().hex[:8]}"
    return {
        "voice_id": clone_id,
        "name": name,
        "status": "success"
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host=HOST, port=PORT)
