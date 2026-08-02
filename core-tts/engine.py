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
    {"id": "Minh Đức", "name": "Minh Đức", "descriptions": ["Male", "Northern", "Expressive"]},
    {"id": "Phạm Tuyên", "name": "Phạm Tuyên", "descriptions": ["Male", "Southern", "News"]},
    {"id": "Thái Sơn", "name": "Thái Sơn", "descriptions": ["Male", "Central", "Natural"]},
    {"id": "Xuân Vĩnh", "name": "Xuân Vĩnh", "descriptions": ["Male", "Northern", "Audiobook"]},
    {"id": "Thanh Bình", "name": "Thanh Bình", "descriptions": ["Male", "Southern", "Warm / Deep"]},
    {"id": "Trúc Ly", "name": "Trúc Ly", "descriptions": ["Female", "Southern", "Natural"]},
    {"id": "Ngọc Linh", "name": "Ngọc Linh", "descriptions": ["Female", "Northern", "Expressive"]},
    {"id": "Đoan Trang", "name": "Đoan Trang", "descriptions": ["Female", "Northern", "Formal"]},
    {"id": "Mai Anh", "name": "Mai Anh", "descriptions": ["Female", "Southern", "Gentle"]},
    {"id": "Thục Đoan", "name": "Thục Đoan", "descriptions": ["Female", "Southern", "Warm"]},
    {"id": "Minh Triết", "name": "Minh Triết", "descriptions": ["Male", "Northern", "Narrative"]},
    {"id": "Thùy Dung", "name": "Thùy Dung", "descriptions": ["Female", "Central", "Natural"]},
    {"id": "Quang Sơn", "name": "Quang Sơn", "descriptions": ["Male", "Northern", "Warm / Deep"]},
    {"id": "Ngọc Trân", "name": "Ngọc Trân", "descriptions": ["Female", "Southern", "Clear"]}
]

def get_preset_voices(model_id: str | None = None):
    """Preset voices, optionally narrowed to one mode.

    The two groups come from different backends: Edge TTS drives `fast`, the local neural
    model drives `standard` and `clone`. Returning all of them for every mode offered the
    user voices that would fail at synthesis time, so each entry now declares which modes
    it belongs to and callers can filter.
    """
    voices = []

    # 1. Fast Edge TTS voices — only reachable through the streaming mode.
    for k, v in FAST_VOICES.items():
        if not any(x["id"] == v for x in voices):
            gender = "Female" if "Nữ" in k or "Female" in k else "Male"
            region = "Southern" if "Hoài Mỹ" in k else "Northern"
            voices.append({
                "id": v,
                "name": k,
                "descriptions": [gender, region, "Natural"],
                "modes": ["fast"]
            })

    # 2. Neural voices — the local model, used by standard and clone.
    for p in DEFAULT_PRESET_VOICES:
        voices.append({**p, "modes": p.get("modes") or ["standard", "clone"]})

    if not model_id:
        return voices

    # An empty modes list means the voice works everywhere, so keep it either way.
    return [v for v in voices if not v.get("modes") or model_id in v["modes"]]

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
