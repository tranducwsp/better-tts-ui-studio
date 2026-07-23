import uuid
import time
import gc
import os
import asyncio
from fastapi import APIRouter, HTTPException, BackgroundTasks, Depends

from core.models import User
from api.auth import get_current_active_user
from core.schemas import FastTTSRequest
from core.state import CoreTTSClient, tasks_db, cleanup_tasks_db
from core.database import update_chunk_status

router = APIRouter()

FAST_VOICES = {
    "Hoài Mỹ (Nữ)": "vi-VN-HoaiMyNeural",
    "Nam Minh (Nam)": "vi-VN-NamMinhNeural",
    "Hoài Mỹ": "vi-VN-HoaiMyNeural",
    "Nam Minh": "vi-VN-NamMinhNeural",
    "vi-VN-HoaiMyNeural": "vi-VN-HoaiMyNeural",
    "vi-VN-NamMinhNeural": "vi-VN-NamMinhNeural"
}

@router.get("/voices")
def get_fast_voices(current_user: User = Depends(get_current_active_user)):
    return ["Hoài Mỹ (Nữ)", "Nam Minh (Nam)"]

@router.post("/synthesize")
async def synthesize_fast_tts(req: FastTTSRequest, background_tasks: BackgroundTasks, current_user: User = Depends(get_current_active_user)):
    if not req.text.strip():
        raise HTTPException(status_code=400, detail="Văn bản trống")
    
    cleanup_tasks_db()
    
    task_id = req.task_id if req.task_id else str(uuid.uuid4())
    job_id = req.job_id
    
    tasks_db[task_id] = {"progress": 0, "status": "processing", "audio_mp3": None, "audio": None, "cancel": False, "created_at": time.time()}
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
            print("Error creating chunk:", e)
        finally:
            db.close()

    def process_task():
        try:
            voice_code = FAST_VOICES.get(req.voice, req.voice)
            
            # Request audio generation from Core TTS Microservice (Port 8001)
            audio_bytes = CoreTTSClient.synthesize(
                text=req.text,
                voice=voice_code,
                speed=req.speed,
                engine="fast"
            )
            
            tasks_db[task_id]["audio_mp3"] = audio_bytes
            tasks_db[task_id]["audio"] = audio_bytes
            
            os.makedirs("storage/temp", exist_ok=True)
            file_path = f"storage/temp/{task_id}.mp3"
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
