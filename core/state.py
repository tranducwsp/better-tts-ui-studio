import time
import gc
import torch
from vieneu import Vieneu

# Khởi tạo mô hình tts chung
torch.set_num_threads(4)
tts = Vieneu(mode="v3turbo", device="cpu", precision="int8")

# Databases & Caches trong RAM
tasks_db = {}
cloned_voices_cache = {}
custom_presets = {}

FAST_VOICES = {
    "Hoài Mỹ (Nữ)": "vi-VN-HoaiMyNeural",
    "Nam Minh (Nam)": "vi-VN-NamMinhNeural"
}

def cleanup_tasks_db():
    """Tự động dọn dẹp các task cũ > 10 phút để giải phóng bộ nhớ RAM"""
    now = time.time()
    expired = [tid for tid, t in list(tasks_db.items()) if now - t.get("created_at", now) > 600]
    for tid in expired:
        tasks_db.pop(tid, None)
    if expired:
        gc.collect()
