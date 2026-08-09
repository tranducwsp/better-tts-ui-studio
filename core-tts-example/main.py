"""
Example TTS Engine — bản tham khảo cho AI engineer tích hợp với Better TTS UI Studio.

Chỉ dùng thư viện chuẩn Python (wave, struct, math) để sinh âm thanh — không cần GPU,
không cần PyTorch, không cần thư viện ngoài. Mục đích là cho thấy contract (endpoints,
manifest, format) mà nền tảng mong đợi, chứ không phải áp đặt cách triển khai.

AI engineer thay thế file này bằng engine thật của mình: Python, C++, Rust, CUDA —
nền tảng không quan tâm, chỉ cần tuân thủ contract.
"""

import os
import uuid
import struct
import math
import asyncio

from fastapi import FastAPI, HTTPException, Response, UploadFile, File, Form
from fastapi.middleware.cors import CORSMiddleware

from schemas import SynthesizeRequest, UniversalManifest, VoiceInfo

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
# Chỉ CORE_PORT là cần thiết cho container. EXAMPLE_DELAY_SEC chỉ dùng cho bản
# example này — engine thật không cần.
PORT = int(os.getenv("CORE_PORT", "8001"))
HOST = os.getenv("CORE_HOST", "0.0.0.0")
EXAMPLE_DELAY_SEC = float(os.getenv("EXAMPLE_DELAY_SEC", "0.0"))

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
    """Sinh âm thanh — trả raw audio binary.

    Bản example này sinh sóng sin ngẫu nhiên dài 5–10 giây.
    Engine thật thay bằng model inference.

    Nền tảng gửi voice_id, speed, engine, và (tuỳ mode) pitch/emotion.
    Nền tảng đọc response body nguyên bản — không bọc JSON, không SSE.
    """
    text_lower = req.text.strip().lower()
    if not text_lower:
        raise HTTPException(status_code=400, detail="Empty text input")

    # Giả lập xử lý (nếu có cấu hình EXAMPLE_DELAY_SEC)
    if EXAMPLE_DELAY_SEC > 0:
        await asyncio.sleep(EXAMPLE_DELAY_SEC)

    # Sinh âm thanh ngẫu nhiên 5–10 giây
    import random
    duration = random.uniform(5.0, 10.0)
    wav_bytes = _generate_wav_bytes(duration)

    return Response(
        content=wav_bytes,
        media_type="audio/wav",
    )


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


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host=HOST, port=PORT)
