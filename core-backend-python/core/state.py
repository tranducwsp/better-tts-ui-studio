import time
import gc
import os
import requests

CORE_TTS_URL = os.getenv("CORE_TTS_URL", "http://localhost:8001")

# Task databases & Caches trong RAM
tasks_db = {}
cloned_voices_cache = {}
custom_presets = {}

def cleanup_tasks_db():
    """Tự động dọn dẹp các task cũ > 10 phút để giải phóng bộ nhớ RAM"""
    now = time.time()
    expired = [tid for tid, t in list(tasks_db.items()) if now - t.get("created_at", now) > 600]
    for tid in expired:
        tasks_db.pop(tid, None)
    if expired:
        gc.collect()

class CoreTTSClient:
    """Client giao tiếp với Core TTS Microservice (Port 8001)"""
    
    @staticmethod
    def get_info():
        try:
            res = requests.get(f"{CORE_TTS_URL}/info", timeout=5)
            if res.ok:
                return res.json()
        except Exception as e:
            print("Lỗi kết nối Core TTS Microservice:", e)
        return None

    @staticmethod
    def get_voices():
        try:
            res = requests.get(f"{CORE_TTS_URL}/voices", timeout=5)
            if res.ok:
                return res.json()
        except Exception as e:
            print("Lỗi lấy danh sách giọng từ Core TTS:", e)
        return []

    @staticmethod
    def synthesize(text: str, voice: str, speed: float = 1.0, engine: str = "standard") -> bytes:
        payload = {
            "text": text,
            "voice_id": voice,
            "speed": speed,
            "engine": engine
        }
        res = requests.post(f"{CORE_TTS_URL}/synthesize", json=payload, timeout=60)
        if not res.ok:
            error_detail = res.text
            try:
                error_detail = res.json().get("detail", res.text)
            except Exception:
                pass
            raise Exception(f"Core TTS Error ({res.status_code}): {error_detail}")
        return res.content

    @staticmethod
    def clone_voice(file_bytes: bytes, filename: str, name: str) -> dict:
        files = {"file": (filename, file_bytes, "audio/wav")}
        data = {"name": name}
        res = requests.post(f"{CORE_TTS_URL}/voices/clone", files=files, data=data, timeout=30)
        if not res.ok:
            raise Exception(f"Lỗi Clone giọng từ Core TTS: {res.text}")
        return res.json()

    @staticmethod
    def delete_voice(voice_id: str) -> dict:
        res = requests.delete(f"{CORE_TTS_URL}/voices/{voice_id}", timeout=10)
        if not res.ok:
            raise Exception(f"Lỗi xóa giọng từ Core TTS: {res.text}")
        return res.json()
