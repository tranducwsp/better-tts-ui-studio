import io
import re
import uuid
import time
import asyncio
import os
import numpy as np
import soundfile as sf
import edge_tts
from vieneu import Vieneu

_vieneu_engine = None

def get_vieneu_engine():
    global _vieneu_engine
    if _vieneu_engine is None:
        print("⏳ Loading ViNeu AI Model Engine...")
        _vieneu_engine = Vieneu(mode="v3turbo", device="cpu", precision="int8")
        print("✅ ViNeu AI Model Engine Loaded Successfully!")
    return _vieneu_engine

# Fast TTS Voices Registry (Full Mapping)
FAST_VOICES = {
    "Hoài Mỹ (Nữ)": "vi-VN-HoaiMyNeural",
    "Nam Minh (Nam)": "vi-VN-NamMinhNeural",
    "Hoài Mỹ": "vi-VN-HoaiMyNeural",
    "Nam Minh": "vi-VN-NamMinhNeural",
    "hoai_my": "vi-VN-HoaiMyNeural",
    "nam_minh": "vi-VN-NamMinhNeural",
    "vi-VN-HoaiMyNeural": "vi-VN-HoaiMyNeural",
    "vi-VN-NamMinhNeural": "vi-VN-NamMinhNeural"
}

# Task Database in RAM
tasks_db = {}
cloned_voices_cache = {}

def cleanup_tasks_db():
    now = time.time()
    expired = [tid for tid, t in list(tasks_db.items()) if now - t.get("created_at", now) > 600]
    for tid in expired:
        tasks_db.pop(tid, None)

DEFAULT_PRESET_VOICES = [
    {"id": "Minh Đức", "name": "Minh Đức", "descriptions": ["Nam", "Miền Bắc", "Truyền cảm"]},
    {"id": "Phạm Tuyên", "name": "Phạm Tuyên", "descriptions": ["Nam", "Miền Nam", "Báo chí"]},
    {"id": "Thái Sơn", "name": "Thái Sơn", "descriptions": ["Nam", "Miền Trung", "Tự nhiên"]},
    {"id": "Xuân Vĩnh", "name": "Xuân Vĩnh", "descriptions": ["Nam", "Miền Bắc", "Truyện đọc"]},
    {"id": "Thanh Bình", "name": "Thanh Bình", "descriptions": ["Nam", "Miền Nam", "Trầm ấm"]},
    {"id": "Trúc Ly", "name": "Trúc Ly", "descriptions": ["Nữ", "Miền Nam", "Tự nhiên"]},
    {"id": "Ngọc Linh", "name": "Ngọc Linh", "descriptions": ["Nữ", "Miền Bắc", "Truyền cảm"]},
    {"id": "Đoan Trang", "name": "Đoan Trang", "descriptions": ["Nữ", "Miền Bắc", "Trang trọng"]},
    {"id": "Mai Anh", "name": "Mai Anh", "descriptions": ["Nữ", "Miền Nam", "Nhẹ nhàng"]},
    {"id": "Thục Đoan", "name": "Thục Đoan", "descriptions": ["Nữ", "Miền Nam", "Ấm áp"]},
    {"id": "Minh Triết", "name": "Minh Triết", "descriptions": ["Nam", "Miền Bắc", "Thuyết minh"]},
    {"id": "Thùy Dung", "name": "Thùy Dung", "descriptions": ["Nữ", "Miền Trung", "Tự nhiên"]},
    {"id": "Quang Sơn", "name": "Quang Sơn", "descriptions": ["Nam", "Miền Bắc", "Trầm ấm"]},
    {"id": "Ngọc Trân", "name": "Ngọc Trân", "descriptions": ["Nữ", "Miền Nam", "Trong trẻo"]}
]

def get_preset_voices():
    voices = []
    # 1. Fast Edge TTS voices
    for k, v in FAST_VOICES.items():
        if not any(x["id"] == v for x in voices):
            gender = "Nữ" if "Nữ" in k else "Nam"
            region = "Miền Nam" if "Hoài Mỹ" in k else "Miền Bắc"
            voices.append({
                "id": v,
                "name": k,
                "descriptions": [gender, region, "Tự nhiên"]
            })

    # 2. Standard voices
    for p in DEFAULT_PRESET_VOICES:
        voices.append(p)

    return voices

def synthesize_standard_sync(text: str, voice: str, speed: float = 1.0) -> bytes:
    if not text.strip().endswith(('.', '?', '!')):
        text = text.strip() + '.'
    
    engine = get_vieneu_engine()
    voice_param = voice
    if voice in cloned_voices_cache:
        voice_data = cloned_voices_cache[voice]
        if "speaker_emb" in voice_data:
            voice_param = voice_data
        elif "path" in voice_data and os.path.exists(voice_data["path"]):
            speaker_emb, ref_codes = engine.encode_reference(voice_data["path"])
            voice_data["speaker_emb"] = speaker_emb
            voice_data["ref_codes"] = ref_codes
            voice_param = voice_data
    elif isinstance(voice, str) and os.path.exists(voice):
        speaker_emb, ref_codes = engine.encode_reference(voice)
        voice_param = {"speaker_emb": speaker_emb, "ref_codes": ref_codes}

    stream_gen = engine.infer_stream(text=text, voice=voice_param, speed=speed)
    audio_chunks = []
    for chunk in stream_gen:
        audio_chunks.append(chunk)
        
    if not audio_chunks:
        raise ValueError("Không tạo được âm thanh từ ViNeu Engine")
        
    full_audio = np.concatenate(audio_chunks)
    out_io = io.BytesIO()
    sf.write(out_io, full_audio, engine.sample_rate, format="WAV")
    return out_io.getvalue()

def get_fast_voice_code(voice: str) -> str:
    if voice in FAST_VOICES:
        return FAST_VOICES[voice]
    clean_voice = voice.split(" — ")[0].strip()
    if clean_voice in FAST_VOICES:
        return FAST_VOICES[clean_voice]
    voice_lower = clean_voice.lower()
    if "nam" in voice_lower or "minh" in voice_lower:
        return "vi-VN-NamMinhNeural"
    return "vi-VN-HoaiMyNeural"

async def synthesize_fast_async(text: str, voice: str, speed: float = 1.0) -> bytes:
    voice_code = get_fast_voice_code(voice)
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
