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

DEFAULT_PRESET_NAMES = [
    'Minh Đức', 'Phạm Tuyên', 'Thái Sơn', 'Xuân Vĩnh', 'Thanh Bình',
    'Trúc Ly', 'Ngọc Linh', 'Đoan Trang', 'Mai Anh', 'Thục Đoan',
    'Minh Triết', 'Thùy Dung', 'Quang Sơn', 'Ngọc Trân'
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
                "type": "fast",
                "language": "vi-VN",
                "gender": gender,
                "region": region,
                "style": "Tự nhiên"
            })

    # 2. Standard voices (Use loaded instance if available, else default list without forcing heavy load)
    if _vieneu_engine is not None:
        try:
            preset_list = _vieneu_engine.list_preset_voices()
            for item in preset_list:
                name = item[0] if isinstance(item, tuple) else str(item)
                id_ = item[1] if isinstance(item, tuple) else str(item)
                voices.append({"id": id_, "name": name, "type": "standard", "language": "vi-VN"})
        except Exception:
            pass
    else:
        for name in DEFAULT_PRESET_NAMES:
            voices.append({"id": name, "name": name, "type": "standard", "language": "vi-VN"})

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
