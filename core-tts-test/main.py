"""
Example TTS Engine — bản tham khảo cho AI engineer tích hợp với Better TTS UI Studio.

Chỉ dùng thư viện chuẩn Python (wave, struct, math) để sinh âm thanh — không cần GPU,
không cần PyTorch, không cần thư viện ngoài. Mục đích là cho thấy contract (endpoints,
manifest, format) mà nền tảng mong đợi, chứ không phải áp đặt cách triển khai.

AI engineer thay thế file này bằng engine thật của mình: Python, C++, Rust, CUDA —
nền tảng không quan tâm, chỉ cần tuân thủ contract.
"""

import os
import time
import uuid
import struct
import math
import asyncio
from typing import Dict, Any

from fastapi import FastAPI, HTTPException, Response, UploadFile, File, Form
from fastapi.middleware.cors import CORSMiddleware

from schemas import SynthesizeRequest, TaskStatusResponse, UniversalManifest, VoiceInfo

app = FastAPI(
    title="Example TTS Engine",
    description="Bản tham khảo minh hoạ contract giữa Engine và Better TTS UI Studio.",
    version="1.0.0",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# ── Cấu hình ──────────────────────────────────────────────────────────────────
# Chỉ CORE_PORT là cần thiết cho container. MOCK_DELAY_SEC chỉ dùng cho bản
# example này — engine thật không cần.
PORT = int(os.getenv("CORE_PORT", "8001"))
HOST = os.getenv("CORE_HOST", "0.0.0.0")
MOCK_DELAY_SEC = float(os.getenv("MOCK_DELAY_SEC", "0.0"))

# ── Bộ nhớ trong RAM (example only) ──────────────────────────────────────────
tasks_db: Dict[str, Dict[str, Any]] = {}

# ── Giọng đọc mẫu ────────────────────────────────────────────────────────────
# Mỗi giọng khai báo `modes` — danh sách mode hỗ trợ — để nền tảng lọc đúng.
# Engine thật tự quyết định danh sách giọng, không cần giống bản này.
EXAMPLE_VOICES = [
    {"id": "Voice A", "name": "Voice A", "descriptions": ["Female", "Northern", "Expressive"],
     "modes": ["standard", "express", "multilingual", "emotion_v2"]},
    {"id": "Voice B", "name": "Voice B", "descriptions": ["Male", "Southern", "News"],
     "modes": ["standard", "express", "multilingual", "emotion_v2"]},
    {"id": "Voice C", "name": "Voice C", "descriptions": ["Male", "Central", "Natural"],
     "modes": ["fast"]},
]


# ── Sinh âm thanh ngẫu nhiên 5–10 giây ──────────────────────────────────────
# Dùng sóng sin (sine wave) ở tần số ngẫu nhiên — không cần thư viện ngoài.
# Engine thật thay bằng model inference; phần này chỉ để example trả được file.

def _generate_wav_bytes(duration_sec: float, sample_rate: int = 24000) -> bytes:
    """Trả về bytes WAV PCM 16-bit mono, độ dài `duration_sec` giây."""
    import random
    freq = random.uniform(220, 880)  # tần số ngẫu nhiên trong khoảng âm thanh nói
    num_samples = int(sample_rate * duration_sec)
    samples = []
    for i in range(num_samples):
        t = i / sample_rate
        # Sóng sin cơ bản + một chút hài bậc 2 để nghe tự nhiên hơn
        val = 0.6 * math.sin(2 * math.pi * freq * t)
        val += 0.2 * math.sin(2 * math.pi * freq * 2 * t)
        val += 0.1 * math.sin(2 * math.pi * freq * 3 * t)
        # Fade-in / fade-out 50ms để tránh lách cách
        fade = min(i, num_samples - 1 - i, int(sample_rate * 0.05)) / (sample_rate * 0.05)
        val *= fade
        pcm = max(-32768, min(32767, int(val * 32767)))
        samples.append(pcm)

    # Ghi WAV header + data
    data_size = num_samples * 2  # 16-bit = 2 bytes/sample
    buf = bytearray()
    # RIFF header
    buf += b'RIFF'
    buf += struct.pack('<I', 36 + data_size)
    buf += b'WAVE'
    # fmt chunk
    buf += b'fmt '
    buf += struct.pack('<IHHIIHH', 16, 1, 1, sample_rate, sample_rate * 2, 2, 16)
    # data chunk
    buf += b'data'
    buf += struct.pack('<I', data_size)
    for s in samples:
        buf += struct.pack('<h', s)
    return bytes(buf)


def _wav_to_mp3_placeholder(wav_bytes: bytes) -> bytes:
    """Bản example không có encoder MP3 — trả nguyên WAV.

    Engine thật dùng lameenc, pydub, hoặc thư viện riêng để chuyển.
    Nền tảng chỉ cần endpoint trả đúng Content-Type.
    """
    return wav_bytes


# ── Endpoints ────────────────────────────────────────────────────────────────

@app.get("/")
def root():
    return {
        "service": "Example TTS Engine",
        "status": "running",
        "docs": "/docs",
    }


@app.get("/info", response_model=UniversalManifest, tags=["Manifest"])
@app.get("/manifest", response_model=UniversalManifest, tags=["Manifest"])
def get_info():
    """Trả manifest — nền tảng đọc để biết engine hỗ trợ gì."""
    return UniversalManifest()


@app.get("/voices", response_model=list[VoiceInfo], tags=["Voices"])
def get_voices(model_id: str | None = None):
    """Trả danh sách giọng preset, lọc theo mode nếu có `model_id`."""
    if model_id is None:
        return [VoiceInfo(**v) for v in EXAMPLE_VOICES]
    return [VoiceInfo(**v) for v in EXAMPLE_VOICES if model_id in v["modes"]]


@app.get("/health", tags=["Health"])
def health_check():
    """Kiểm tra engine còn sống — nền tảng gọi định kỳ."""
    return {"status": "healthy"}


@app.post("/synthesize", tags=["Synthesis"])
async def synthesize(req: SynthesizeRequest):
    """Sinh âm thanh — trả file WAV hoặc MP3.

    Bản example này sinh sóng sin ngẫu nhiên dài 5–10 giây.
    Engine thật thay bằng model inference.
    """
    text_lower = req.text.strip().lower()
    if not text_lower:
        raise HTTPException(status_code=400, detail="Empty text input")

    task_id = req.task_id or str(uuid.uuid4())

    # Giả lập xử lý (nếu có cấu hình MOCK_DELAY_SEC)
    if MOCK_DELAY_SEC > 0:
        await asyncio.sleep(MOCK_DELAY_SEC)

    # Sinh âm thanh ngẫu nhiên 5–10 giây
    import random
    duration = random.uniform(5.0, 10.0)
    wav_bytes = _generate_wav_bytes(duration)

    if req.output_format == "mp3":
        audio_bytes = _wav_to_mp3_placeholder(wav_bytes)
        media_type = "audio/mpeg"
    else:
        audio_bytes = wav_bytes
        media_type = "audio/wav"

    tasks_db[task_id] = {
        "status": "done",
        "progress": 100,
        "audio": audio_bytes,
        "created_at": time.time(),
    }

    return Response(
        content=audio_bytes,
        media_type=media_type,
        headers={"X-Task-ID": task_id},
    )


@app.get("/tasks/{task_id}", response_model=TaskStatusResponse, tags=["Tasks"])
def get_task_status(task_id: str):
    """Trả trạng thái task — nền tảng dùng cho SSE polling."""
    return TaskStatusResponse(
        task_id=task_id,
        status="done",
        progress=100,
        audio_url=f"/tasks/{task_id}/audio",
    )


@app.get("/tasks/{task_id}/audio", tags=["Tasks"])
def get_task_audio(task_id: str, format: str = "wav"):
    """Trả file âm thanh của task — nền tảng gọi khi SSE nhận được 'done'."""
    audio_bytes = STATIC_MP3_BYTES if format.lower() == "mp3" else STATIC_WAV_BYTES
    media_type = "audio/mpeg" if format.lower() == "mp3" else "audio/wav"
    return Response(content=audio_bytes, media_type=media_type)


@app.delete("/tasks/{task_id}", tags=["Tasks"])
def cancel_task(task_id: str):
    """Huỷ task — nền tảng gọi khi người dùng bấm Cancel."""
    return {"message": "Cancellation requested", "task_id": task_id}


@app.post("/voices/clone", tags=["Voice Cloning"])
async def clone_voice(file: UploadFile = File(...), name: str = Form(...)):
    """Nhận file âm thanh tham chiếu và trả voice_id mới.

    Bản example chỉ trả ID giả. Engine thật dùng model speaker adaptation.
    """
    clone_id = f"clone_example_{uuid.uuid4().hex[:8]}"
    return {
        "voice_id": clone_id,
        "name": name,
        "status": "success",
    }


# ── Pre-generate một WAV mẫu cho endpoint /tasks/{id}/audio ────────────────
# (để không phải sinh lại mỗi lần GET)
STATIC_WAV_BYTES = _generate_wav_bytes(7.0, 24000)
STATIC_MP3_BYTES = _wav_to_mp3_placeholder(STATIC_WAV_BYTES)


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host=HOST, port=PORT)
