"""
Example TTS Engine — bản tham khảo cho AI engineer tích hợp với Better TTS UI Studio.

Mục đích: Cho thấy contract (endpoints, manifest, format) mà nền tảng mong đợi.
AI engineer chỉ cần giữ đúng contract này và thay bằng model inference thực tế (Python, C++, PyTorch, CUDA...).
"""

import os
import uuid
import asyncio
import logging

from fastapi import FastAPI, HTTPException, Response, UploadFile, File, Form
from fastapi.middleware.cors import CORSMiddleware

from schemas import SynthesizeRequest, UniversalManifest, VoiceInfo
from utils.audio_utils import generate_dummy_wav

logger = logging.getLogger("ExampleTTSEngine")

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
PORT = int(os.getenv("CORE_PORT", "8001"))
HOST = os.getenv("CORE_HOST", "0.0.0.0")
EXAMPLE_DELAY_SEC = float(os.getenv("EXAMPLE_DELAY_SEC", "0.0"))

# ── Giọng đọc theo từng Model / Mode ──────────────────────────────────────────
# Thực tế mỗi Model AI (FastTTS, StandardTTS...) sẽ có danh sách giọng (speakers) riêng.
MODEL_VOICES = {
    "fast": [
        {"id": "hoai_my", "name": "Hoài Mỹ", "descriptions": ["Nữ", "Miền Bắc", "Tự nhiên"]},
        {"id": "nam_minh", "name": "Nam Minh", "descriptions": ["Nam", "Miền Nam", "Bản tin"]},
    ],
    "standard": [
        {"id": "hoai_my", "name": "Hoài Mỹ", "descriptions": ["Nữ", "Miền Bắc", "Tự nhiên"]},
        {"id": "nam_minh", "name": "Nam Minh", "descriptions": ["Nam", "Miền Nam", "Bản tin"]},
        {"id": "thu_hien", "name": "Thu Hiền", "descriptions": ["Nữ", "Miền Trung", "Dịu dàng"]},
    ],
}


# ── HTTP REST Endpoints ───────────────────────────────────────────────────────

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
    """Trả manifest — nền tảng đọc để biết engine hỗ trợ những UI slider / emotion / mode gì."""
    return UniversalManifest()


@app.get("/voices", response_model=list[VoiceInfo], tags=["Voices"])
def get_voices(model_id: str | None = None):
    """Trả danh sách giọng preset cho model_id được yêu cầu."""
    if not model_id or model_id == "all":
        seen = set()
        all_voices = []
        for voices in MODEL_VOICES.values():
            for v in voices:
                if v["id"] not in seen:
                    seen.add(v["id"])
                    all_voices.append(v)
        return [VoiceInfo(**v) for v in all_voices]

    voices = MODEL_VOICES.get(model_id, [])
    return [VoiceInfo(**v) for v in voices]


@app.get("/health", tags=["Health"])
def health_check():
    """Kiểm tra engine còn sống — nền tảng gọi định kỳ."""
    return {"status": "healthy"}


@app.post("/synthesize", tags=["Synthesis"])
async def synthesize(req: SynthesizeRequest):
    """Sinh âm thanh — nhận voice_id, speed, text... và trả raw WAV binary."""
    text_lower = req.text.strip().lower()
    if not text_lower:
        raise HTTPException(status_code=400, detail="Empty text input")

    if EXAMPLE_DELAY_SEC > 0:
        await asyncio.sleep(EXAMPLE_DELAY_SEC)

    # Hàm sinh audio PCM ngẫu nhiên phục vụ test
    import random
    duration = random.uniform(5.0, 10.0)
    wav_bytes = generate_dummy_wav(duration)

    return Response(
        content=wav_bytes,
        media_type="audio/wav",
    )


@app.post("/voices/clone", tags=["Voice Cloning"])
async def clone_voice(file: UploadFile = File(...), name: str = Form(...)):
    """Nhận file âm thanh tham chiếu và trả voice_id mới."""
    clone_id = f"clone_example_{uuid.uuid4().hex[:8]}"
    return {
        "voice_id": clone_id,
        "name": name,
        "status": "success",
    }


if __name__ == "__main__":
    # Khởi chạy song song gRPC Server nếu được cấu hình GRPC_PORT
    grpc_port = int(os.getenv("GRPC_PORT", "0"))
    if grpc_port > 0:
        from grpc_server import serve_grpc
        serve_grpc(port=grpc_port)
        logger.info("gRPC server started on port %d", grpc_port)

    import uvicorn
    uvicorn.run(app, host=HOST, port=PORT)
