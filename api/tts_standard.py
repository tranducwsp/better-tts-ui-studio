import io
import uuid
import time
import gc
import asyncio
import os
import numpy as np
import soundfile as sf
from fastapi import APIRouter, HTTPException, BackgroundTasks, Depends

from core.models import User
from api.auth import get_current_active_user
from core.schemas import TTSRequest
from core.state import tts, tasks_db, custom_presets, cleanup_tasks_db
from core.database import update_chunk_status

router = APIRouter()

@router.get("/voices")
def get_voices(current_user: User = Depends(get_current_active_user)):
    """Trả về danh sách các giọng AI có sẵn mặc định + Custom"""
    voices = tts.list_preset_voices()
    return list(custom_presets.keys()) + voices

@router.post("/synthesize")
async def synthesize_audio(req: TTSRequest, background_tasks: BackgroundTasks, current_user: User = Depends(get_current_active_user)):
    """Gửi text, tạo task chạy ngầm và trả về task_id"""
    if not req.text.strip():
        raise HTTPException(status_code=400, detail="Văn bản trống")
    
    # Chống ảo giác VITS
    if not req.text.strip().endswith(('.', '?', '!')):
        req.text = req.text.strip() + '.'
        
    cleanup_tasks_db()
    task_id = req.task_id if req.task_id else str(uuid.uuid4())
    job_id = req.job_id
    
    tasks_db[task_id] = {"progress": 0, "status": "processing", "audio": None, "cancel": False, "created_at": time.time()}
    loop = asyncio.get_running_loop()
    
    if job_id and req.chunk_index is not None and req.total_chunks is not None:
        from core.database import SessionLocal
        from core.models import TTSChunk
        db = SessionLocal()
        try:
            chunk = TTSChunk(
                id=task_id,
                job_id=job_id,
                chunk_index=req.chunk_index,
                text=req.text,
                status="processing"
            )
            db.add(chunk)
            db.commit()
        except Exception as e:
            db.rollback()
        finally:
            db.close()
    
    def process_task():
        try:
            estimated_chunks = max(1, len(req.text) / (4.5 * req.speed))
            
            voice_param = custom_presets.get(req.voice, req.voice)
            
            stream_gen = tts.infer_stream(text=req.text, voice=voice_param, speed=req.speed)
            audio_chunks = []
            
            for i, chunk in enumerate(stream_gen):
                if tasks_db[task_id].get("cancel"):
                    if "queue" in tasks_db[task_id]:
                        loop.call_soon_threadsafe(tasks_db[task_id]["queue"].put_nowait, {"status": "cancelled"})
                    return
                audio_chunks.append(chunk)
                progress = min(99, int((i / estimated_chunks) * 100))
                tasks_db[task_id]["progress"] = max(tasks_db[task_id]["progress"], progress)
                
                if "queue" in tasks_db[task_id]:
                    loop.call_soon_threadsafe(tasks_db[task_id]["queue"].put_nowait, {"status": "processing", "progress": tasks_db[task_id]["progress"]})
                
            if tasks_db[task_id].get("cancel"):
                return
                
            file_path = None
            if audio_chunks:
                full_audio = np.concatenate(audio_chunks)
                out_io = io.BytesIO()
                sf.write(out_io, full_audio, tts.sample_rate, format="WAV")
                tasks_db[task_id]["audio"] = out_io.getvalue()
                
                try:
                    mp3_io = io.BytesIO()
                    sf.write(mp3_io, full_audio, tts.sample_rate, format="MP3")
                    tasks_db[task_id]["audio_mp3"] = mp3_io.getvalue()
                except Exception:
                    pass
                
                os.makedirs("storage/temp", exist_ok=True)
                file_path = f"storage/temp/{task_id}.wav"
                with open(file_path, "wb") as f:
                    f.write(tasks_db[task_id]["audio"])
            
            if job_id:
                update_chunk_status(task_id, "done", file_path)
                
            tasks_db[task_id]["progress"] = 100
            tasks_db[task_id]["status"] = "done"
            if "queue" in tasks_db[task_id]:
                loop.call_soon_threadsafe(tasks_db[task_id]["queue"].put_nowait, {"status": "done"})
        except Exception as e:
            tasks_db[task_id]["status"] = "error"
            tasks_db[task_id]["error"] = str(e)
            if job_id:
                update_chunk_status(task_id, "error", None, str(e))
            if "queue" in tasks_db[task_id]:
                loop.call_soon_threadsafe(tasks_db[task_id]["queue"].put_nowait, {"status": "error", "error": str(e)})
        finally:
            gc.collect()
            
    background_tasks.add_task(process_task)
    return {"task_id": task_id}
