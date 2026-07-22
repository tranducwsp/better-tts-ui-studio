import os
import io
import uuid
import time
import gc
import asyncio
import tempfile
import numpy as np
import soundfile as sf
from fastapi import APIRouter, HTTPException, UploadFile, File, Form, Depends, BackgroundTasks
from sqlalchemy.orm import Session

from core.schemas import CloneSynthesizeRequest
from core.state import tts, tasks_db, cloned_voices_cache, cleanup_tasks_db
from core.models import UserVoice, User
from core.database import get_db, register_job_and_chunk, update_chunk_status
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
        
        with open(file_path, "wb") as f:
            f.write(await file.read())
            
        speaker_emb, ref_codes = tts.encode_reference(file_path)
        cloned_voices_cache[clone_id] = {
            "speaker_emb": speaker_emb,
            "ref_codes": ref_codes
        }
        
        user_voice = UserVoice(
            id=clone_id,
            user_id=current_user.id,
            name=name,
            gender=gender,
            region=region,
            style=style,
            file_path=file_path
        )
        db.add(user_voice)
        db.commit()
        
        return {"clone_id": clone_id, "message": "Clone giọng thành công!"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@router.post("/upload-temp")
async def clone_voice_temp(
    file: UploadFile = File(...),
    current_user: User = Depends(get_current_active_user)
):
    try:
        temp_dir = "storage/temp"
        os.makedirs(temp_dir, exist_ok=True)
        clone_id = f"temp_{uuid.uuid4()}"
        ext = file.filename.split('.')[-1] if '.' in file.filename else 'wav'
        file_path = f"{temp_dir}/{clone_id}.{ext}"
        
        with open(file_path, "wb") as f:
            f.write(await file.read())
            
        speaker_emb, ref_codes = tts.encode_reference(file_path)
        cloned_voices_cache[clone_id] = {
            "speaker_emb": speaker_emb,
            "ref_codes": ref_codes
        }
        
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
    if clone_id in cloned_voices_cache:
        del cloned_voices_cache[clone_id]
    db.delete(voice)
    db.commit()
    return {"message": "Đã xóa giọng"}

@router.post("/synthesize")
async def synthesize_clone(req: CloneSynthesizeRequest, background_tasks: BackgroundTasks, db: Session = Depends(get_db), current_user: User = Depends(get_current_active_user)):
    voice = db.query(UserVoice).filter(UserVoice.id == req.clone_id).first()
    if not voice and req.clone_id not in cloned_voices_cache:
        raise HTTPException(status_code=404, detail="Không tìm thấy ID giọng clone này")
        
    cleanup_tasks_db()
    task_id = req.task_id if req.task_id else str(uuid.uuid4())
    job_id = req.job_id
    
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
            if req.clone_id not in cloned_voices_cache:
                speaker_emb, ref_codes = tts.encode_reference(voice.file_path)
                cloned_voices_cache[req.clone_id] = {
                    "speaker_emb": speaker_emb,
                    "ref_codes": ref_codes
                }
                
            voice_data = cloned_voices_cache[req.clone_id]
            estimated_chunks = max(1, len(req.text) / (4.5 * req.speed))
            
            stream_gen = tts.infer_stream(
                text=req.text, 
                voice=voice_data,
                speed=req.speed
            )
            
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
