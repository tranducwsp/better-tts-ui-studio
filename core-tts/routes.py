import os
import uuid
import time
import asyncio
from fastapi import APIRouter, HTTPException, BackgroundTasks, Response, UploadFile, File, Form
from fastapi.responses import FileResponse

from schemas import (
    SynthesizeRequest,
    TaskStatusResponse,
    CoreInfoResponse,
    VoiceInfo
)
from engine import (
    get_preset_voices,
    synthesize_standard_sync,
    synthesize_fast_async,
    vieneu_engine,
    tasks_db,
    cloned_voices_cache,
    cleanup_tasks_db
)
from config import STORAGE_DIR

router = APIRouter()

@router.get("/info", response_model=CoreInfoResponse)
def get_core_info():
    """2.1 Capabilities & Manifest API"""
    return CoreInfoResponse()

@router.get("/voices", response_model=list[VoiceInfo])
def get_voices():
    """2.2 Voices Metadata API"""
    voices = get_preset_voices()
    return [VoiceInfo(**v) for v in voices]

@router.get("/health")
def health_check():
    """2.3 Healthcheck & Metrics API"""
    import torch
    return {
        "status": "healthy",
        "gpu_available": torch.cuda.is_available(),
        "vram_used_mb": 0,
        "active_jobs": len([t for t in tasks_db.values() if t.get("status") == "processing"])
    }

@router.post("/synthesize")
async def synthesize(req: SynthesizeRequest, background_tasks: BackgroundTasks):
    """2.4 Core Synthesis API (Supports both synchronous binary WAV or Async Task ID)"""
    if not req.text.strip():
        raise HTTPException(status_code=400, detail="Văn bản trống")
        
    cleanup_tasks_db()
    task_id = req.task_id if req.task_id else str(uuid.uuid4())
    target_voice = req.voice_id or req.voice or "Minh Đức"
    
    # Rút gọn tên giọng nếu chứa dấu gạch ngang mô tả
    if " — " in target_voice:
        target_voice = target_voice.split(" — ")[0].strip()

    engine_type = req.engine if req.engine else ("fast" if "Neural" in target_voice or "Hoài Mỹ" in target_voice or "Nam Minh" in target_voice else "standard")

    tasks_db[task_id] = {
        "progress": 0,
        "status": "processing",
        "audio": None,
        "audio_mp3": None,
        "cancel": False,
        "created_at": time.time()
    }

    # If small single text request, synthesize synchronously
    try:
        if engine_type == "fast":
            audio_bytes = await synthesize_fast_async(req.text, target_voice, req.speed)
            tasks_db[task_id]["audio"] = audio_bytes
            tasks_db[task_id]["audio_mp3"] = audio_bytes
            tasks_db[task_id]["status"] = "done"
            tasks_db[task_id]["progress"] = 100
            
            # File Response
            file_path = os.path.join(STORAGE_DIR, f"{task_id}.mp3")
            with open(file_path, "wb") as f:
                f.write(audio_bytes)
                
            return Response(content=audio_bytes, media_type="audio/mpeg", headers={"X-Task-ID": task_id})
        else:
            audio_bytes = synthesize_standard_sync(req.text, target_voice, req.speed)
            tasks_db[task_id]["audio"] = audio_bytes
            tasks_db[task_id]["status"] = "done"
            tasks_db[task_id]["progress"] = 100
            
            file_path = os.path.join(STORAGE_DIR, f"{task_id}.wav")
            with open(file_path, "wb") as f:
                f.write(audio_bytes)
                
            return Response(content=audio_bytes, media_type="audio/wav", headers={"X-Task-ID": task_id})
    except Exception as e:
        tasks_db[task_id]["status"] = "error"
        tasks_db[task_id]["error"] = str(e)
        raise HTTPException(status_code=500, detail=str(e))

@router.get("/tasks/{task_id}", response_model=TaskStatusResponse)
def get_task_status(task_id: str):
    """2.5 Task Status API"""
    if task_id not in tasks_db:
        raise HTTPException(status_code=404, detail="Không tìm thấy task")
        
    t = tasks_db[task_id]
    return TaskStatusResponse(
        task_id=task_id,
        status=t["status"],
        progress=t.get("progress", 0),
        audio_url=f"/tasks/{task_id}/audio" if t["status"] == "done" else None,
        error=t.get("error")
    )

@router.get("/tasks/{task_id}/audio")
def get_task_audio(task_id: str, format: str = "wav"):
    """Get Audio Output Bytes by Task ID"""
    if task_id not in tasks_db or tasks_db[task_id]["status"] != "done":
        raise HTTPException(status_code=404, detail="Audio chưa sẵn sàng")
        
    t = tasks_db[task_id]
    if format.lower() == "mp3" and t.get("audio_mp3"):
        return Response(content=t["audio_mp3"], media_type="audio/mpeg")
    return Response(content=t["audio"], media_type="audio/wav")

@router.delete("/tasks/{task_id}")
def cancel_task(task_id: str):
    """2.5 Task Cancellation API (Interrupt CUDA Inference)"""
    if task_id in tasks_db:
        tasks_db[task_id]["cancel"] = True
        tasks_db[task_id]["status"] = "cancelled"
    return {"message": "Đã yêu cầu hủy task", "task_id": task_id}

@router.post("/voices/clone")
async def clone_voice(file: UploadFile = File(...), name: str = Form(...)):
    """3. Voice Cloning Extension API"""
    try:
        content = await file.read()
        clone_id = f"clone_{uuid.uuid4().hex[:8]}"
        file_path = os.path.join(STORAGE_DIR, f"{clone_id}.wav")
        with open(file_path, "wb") as f:
            f.write(content)
            
        speaker_emb, ref_codes = vieneu_engine.encode_reference(file_path)
        cloned_voices_cache[clone_id] = {
            "id": clone_id,
            "name": name,
            "path": file_path,
            "speaker_emb": speaker_emb,
            "ref_codes": ref_codes
        }
        return {"voice_id": clone_id, "name": name}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@router.delete("/voices/{voice_id}")
def delete_voice(voice_id: str):
    """Delete Custom Voice Embedding API"""
    if voice_id in cloned_voices_cache:
        info = cloned_voices_cache.pop(voice_id)
        if os.path.exists(info["path"]):
            os.remove(info["path"])
        return {"success": True, "deleted_voice_id": voice_id}
    raise HTTPException(status_code=404, detail="Không tìm thấy giọng clone")
