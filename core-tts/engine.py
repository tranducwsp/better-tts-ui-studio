import io
import re
import uuid
import time
import asyncio
import numpy as np
import soundfile as sf
import edge_tts
from vieneu import Vieneu

# Initialize ViNeu AI Model Engine (CPU/Int8 or CUDA)
vieneu_engine = Vieneu(mode="v3turbo", device="cpu", precision="int8")

# Fast TTS Voices Registry
FAST_VOICES = {
    "Hoài Mỹ (Nữ)": "vi-VN-HoaiMyNeural",
    "Nam Minh (Nam)": "vi-VN-NamMinhNeural",
    "hoai_my": "vi-VN-HoaiMyNeural",
    "nam_minh": "vi-VN-NamMinhNeural"
}

# Task Database in RAM
tasks_db = {}
cloned_voices_cache = {}

def cleanup_tasks_db():
    now = time.time()
    expired = [tid for tid, t in list(tasks_db.items()) if now - t.get("created_at", now) > 600]
    for tid in expired:
        tasks_db.pop(tid, None)

def get_preset_voices():
    preset_list = vieneu_engine.list_preset_voices()
    voices = []
    for p in preset_list:
        voices.append({
            "id": p,
            "name": p,
            "type": "standard",
            "language": "vi-VN"
        })
    for k, v in FAST_VOICES.items():
        if not any(x["id"] == v for x in voices):
            voices.append({
                "id": k,
                "name": k,
                "type": "fast",
                "language": "vi-VN"
            })
    return voices

def synthesize_standard_sync(text: str, voice: str, speed: float = 1.0) -> bytes:
    if not text.strip().endswith(('.', '?', '!')):
        text = text.strip() + '.'
    
    stream_gen = vieneu_engine.infer_stream(text=text, voice=voice, speed=speed)
    audio_chunks = []
    for chunk in stream_gen:
        audio_chunks.append(chunk)
        
    if not audio_chunks:
        raise ValueError("Không tạo được âm thanh từ ViNeu Engine")
        
    full_audio = np.concatenate(audio_chunks)
    out_io = io.BytesIO()
    sf.write(out_io, full_audio, vieneu_engine.sample_rate, format="WAV")
    return out_io.getvalue()

async def synthesize_fast_async(text: str, voice: str, speed: float = 1.0) -> bytes:
    voice_code = FAST_VOICES.get(voice, voice if "Neural" in voice else "vi-VN-HoaiMyNeural")
    rate_percent = int((speed - 1.0) * 100)
    rate_str = f"{rate_percent:+d}%" if rate_percent != 0 else None

    if rate_str:
        communicate = edge_tts.Communicate(text, voice_code, rate=rate_str)
    else:
        communicate = edge_tts.Communicate(text, voice_code)

    audio_bytes = b""
    async for chunk in communicate.stream():
        if chunk["type"] == "audio":
            audio_bytes += chunk["data"]
            
    if not audio_bytes:
        raise ValueError("No audio was received from FastTTS Engine")
        
    return audio_bytes
