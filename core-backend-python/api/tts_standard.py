import uuid
import time
import gc
import os
import asyncio
from fastapi import APIRouter, HTTPException, BackgroundTasks, Depends

from core.models import User
from api.auth import get_current_active_user
from core.schemas import TTSRequest
from core.state import CoreTTSClient, tasks_db, custom_presets, cleanup_tasks_db
from core.database import update_chunk_status

router = APIRouter()

@router.get("/voices")
def get_voices(current_user: User = Depends(get_current_active_user)):
    """Trả về danh sách các giọng AI từ Core TTS Microservice"""
    voices_from_core = CoreTTSClient.get_voices()
    if voices_from_core:
        return [v["name"] for v in voices_from_core]
    return ["Minh Đức", "Hoài Mỹ"]

@router.post("/synthesize")
async def synthesize_audio(req: TTSRequest, background_tasks: BackgroundTasks, current_user: User = Depends(get_current_active_user)):
    """Gửi text chunk tới Core TTS Microservice, lưu DB Lịch sử và trả về task_id"""
    if not req.text.strip():
        raise HTTPException(status_code=400, detail="Văn bản trống")
        
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
            voice_param = custom_presets.get(req.voice, req.voice)
            
            # Request audio generation from Core TTS Microservice (Port 8001)
            audio_bytes = CoreTTSClient.synthesize(
                text=req.text,
                voice=voice_param,
                speed=req.speed,
                engine="standard"
            )
            
            tasks_db[task_id]["audio"] = audio_bytes
            tasks_db[task_id]["audio_mp3"] = audio_bytes
            
            os.makedirs("storage/temp", exist_ok=True)
            file_path = f"storage/temp/{task_id}.wav"
            with open(file_path, "wb") as f:
                f.write(audio_bytes)
            
            if job_id:
                update_chunk_status(task_id, "done", file_path)
                
            tasks_db[task_id]["progress"] = 100
            tasks_db[task_id]["status"] = "done"
            
            # Notify SSE Queue immediately
            if "queue" in tasks_db[task_id]:
                loop.call_soon_threadsafe(tasks_db[task_id]["queue"].put_nowait, {"status": "done", "progress": 100})
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
