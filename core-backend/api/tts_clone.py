import os
import uuid
import time
import gc
import asyncio
from fastapi import APIRouter, HTTPException, UploadFile, File, Form, Depends, BackgroundTasks
from sqlalchemy.orm import Session

from core.schemas import CloneSynthesizeRequest
from core.state import CoreTTSClient, tasks_db, cloned_voices_cache, cleanup_tasks_db
from core.models import UserVoice, User
from core.database import get_db, update_chunk_status
from api.auth import get_current_active_user

router = APIRouter()

@router.post("/upload")
async def clone_voice(
    name: str = Form(...),
    gender: str = Form(None),
    region: str = Form(None),
    style: str = Form(None),
    file: UploadFile = File(...),
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_active_user)
):
    try:
        user_dir = f"storage/{current_user.id}/voice"
        os.makedirs(user_dir, exist_ok=True)
        clone_id = str(uuid.uuid4())
        ext = file.filename.split('.')[-1] if '.' in file.filename else 'wav'
        file_path = f"{user_dir}/{clone_id}.{ext}"
        
        content = await file.read()
        with open(file_path, "wb") as f:
            f.write(content)
            
        # Send audio sample to Core TTS Microservice to extract embedding
        res = CoreTTSClient.clone_voice(content, file.filename, name)
        core_clone_id = res.get("voice_id", clone_id)
        
        user_voice = UserVoice(
            id=core_clone_id,
            user_id=current_user.id,
            name=name,
            gender=gender,
            region=region,
            style=style,
            file_path=file_path
        )
        db.add(user_voice)
        db.commit()
        
        return {"clone_id": core_clone_id, "message": "Clone giọng thành công!"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@router.post("/upload-temp")
async def clone_voice_temp(
    file: UploadFile = File(...),
    current_user: User = Depends(get_current_active_user)
):
    try:
        content = await file.read()
        res = CoreTTSClient.clone_voice(content, file.filename, "temp_voice")
        clone_id = res.get("voice_id", f"temp_{uuid.uuid4()}")
        return {"clone_id": clone_id, "message": "Nạp giọng tạm thành công!"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@router.get("/voices")
def get_user_voices(db: Session = Depends(get_db), current_user: User = Depends(get_current_active_user)):
    voices = db.query(UserVoice).filter(UserVoice.user_id == current_user.id).all()
    return [{
        "id": v.id,
        "name": v.name,
        "gender": v.gender,
        "region": v.region,
        "style": v.style,
        "created_at": v.created_at
    } for v in voices]

@router.delete("/voices/{clone_id}")
def delete_user_voice(clone_id: str, db: Session = Depends(get_db), current_user: User = Depends(get_current_active_user)):
    voice = db.query(UserVoice).filter(UserVoice.id == clone_id, UserVoice.user_id == current_user.id).first()
    if not voice:
        raise HTTPException(status_code=404, detail="Không tìm thấy giọng")
    if os.path.exists(voice.file_path):
        os.remove(voice.file_path)
        
    try:
        CoreTTSClient.delete_voice(clone_id)
    except Exception:
        pass
        
    db.delete(voice)
    db.commit()
    return {"message": "Đã xóa giọng"}

@router.post("/synthesize")
async def synthesize_clone(req: CloneSynthesizeRequest, background_tasks: BackgroundTasks, db: Session = Depends(get_db), current_user: User = Depends(get_current_active_user)):
    cleanup_tasks_db()
    task_id = req.task_id if req.task_id else str(uuid.uuid4())
    job_id = req.job_id
    target_clone_id = req.clone_id or req.voice
    
    tasks_db[task_id] = {"progress": 0, "status": "processing", "audio": None, "cancel": False, "created_at": time.time()}
    loop = asyncio.get_running_loop()

    if job_id and req.chunk_index is not None and req.total_chunks is not None:
        from core.database import SessionLocal
        from core.models import TTSChunk
        db_session = SessionLocal()
        try:
            chunk = TTSChunk(
                id=task_id,
                job_id=job_id,
                chunk_index=req.chunk_index,
                text=req.text,
                status="processing"
            )
            db_session.add(chunk)
            db_session.commit()
        except Exception as e:
            db.rollback()
        finally:
            db_session.close()
    
    def process_task():
        try:
            # Proxy synthesis request to Core TTS Microservice
            audio_bytes = CoreTTSClient.synthesize(
                text=req.text,
                voice=target_clone_id,
                speed=req.speed,
                engine="clone"
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
