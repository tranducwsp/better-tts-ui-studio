from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from sqlalchemy import func
from typing import List, Optional
from pydantic import BaseModel
from datetime import datetime

from core.database import get_db
from core.models import TTSJob, TTSChunk, User
from api.auth import get_current_active_user, get_current_admin_user

router = APIRouter()

@router.get("/history")
def get_user_history(db: Session = Depends(get_db), current_user: User = Depends(get_current_active_user)):
    jobs = db.query(TTSJob).filter(TTSJob.user_id == current_user.id).order_by(TTSJob.created_at.desc()).all()
    
    result = []
    for job in jobs:
        if job.text:
            short_text = job.text[:50] + "..." if len(job.text) > 50 else job.text
        else:
            first_chunk = next((c for c in job.chunks if c.chunk_index == 0), None)
            if not first_chunk and job.chunks:
                first_chunk = job.chunks[0]
            short_text = first_chunk.text if first_chunk else "Chưa có nội dung"
        
        unique_indexes = set(c.chunk_index for c in job.chunks)
        done_indexes = set(c.chunk_index for c in job.chunks if c.status == "done")
        done_chunks = len(done_indexes)
        
        # Sửa lỗi hiển thị cho các job cũ bị lưu total_chunks = 1
        actual_total = max(job.total_chunks, len(unique_indexes))
        
        # Format time ago
        delta = datetime.utcnow() - job.created_at
        if delta.days > 0:
            time_ago = f"{delta.days} ngày trước"
        elif delta.seconds // 3600 > 0:
            time_ago = f"{delta.seconds // 3600} giờ trước"
        elif delta.seconds // 60 > 0:
            time_ago = f"{delta.seconds // 60} phút trước"
        else:
            time_ago = "Vừa xong"
            
        result.append({
            "job_id": job.id,
            "engine": job.engine,
            "voice": job.voice,
            "speed": job.speed,
            "text": short_text,
            "time_ago": time_ago,
            "progress": f"{done_chunks}/{actual_total}",
            "is_complete": done_chunks == actual_total
            # KHÔNG trả về chunks array ở đây để API cực nhẹ
        })
        
    return result

@router.get("/admin/users/{user_id}/history")
def get_user_history_admin(user_id: str, db: Session = Depends(get_db), admin: User = Depends(get_current_admin_user)):
    jobs = db.query(TTSJob).filter(TTSJob.user_id == user_id).order_by(TTSJob.created_at.desc()).all()
    
    result = []
    for job in jobs:
        if job.text:
            short_text = job.text[:50] + "..." if len(job.text) > 50 else job.text
        else:
            first_chunk = next((c for c in job.chunks if c.chunk_index == 0), None)
            if not first_chunk and job.chunks:
                first_chunk = job.chunks[0]
            short_text = first_chunk.text if first_chunk else "Chưa có nội dung"
        
        unique_indexes = set(c.chunk_index for c in job.chunks)
        done_indexes = set(c.chunk_index for c in job.chunks if c.status == "done")
        done_chunks = len(done_indexes)
        
        actual_total = max(job.total_chunks, len(unique_indexes))
        
        delta = datetime.utcnow() - job.created_at
        if delta.days > 0:
            time_ago = f"{delta.days} ngày trước"
        elif delta.seconds // 3600 > 0:
            time_ago = f"{delta.seconds // 3600} giờ trước"
        elif delta.seconds // 60 > 0:
            time_ago = f"{delta.seconds // 60} phút trước"
        else:
            time_ago = "Vừa xong"
            
        result.append({
            "job_id": job.id,
            "engine": job.engine,
            "voice": job.voice,
            "speed": job.speed,
            "text": short_text,
            "time_ago": time_ago,
            "progress": f"{done_chunks}/{actual_total}",
            "is_complete": done_chunks == actual_total
        })
        
    return result

@router.get("/history/{job_id}")
def get_job_detail(job_id: str, db: Session = Depends(get_db), current_user: User = Depends(get_current_active_user)):
    job = db.query(TTSJob).filter(TTSJob.id == job_id).first()
    if not job:
        from fastapi import HTTPException
        raise HTTPException(status_code=404, detail="Job not found")
        
    if job.user_id != current_user.id and current_user.role != "admin":
        from fastapi import HTTPException
        raise HTTPException(status_code=403, detail="Forbidden")
        
    best_chunks = {}
    status_priority = {"done": 4, "processing": 3, "pending": 2, "error": 1}
    for c in job.chunks:
        if c.chunk_index not in best_chunks or status_priority.get(c.status, 0) > status_priority.get(best_chunks[c.chunk_index].status, 0):
            best_chunks[c.chunk_index] = c
            
    chunks = sorted(best_chunks.values(), key=lambda c: c.chunk_index)
    full_text = job.text if job.text else " ".join([c.text for c in chunks if c.text])
    
    return {
        "job_id": job.id,
        "engine": job.engine,
        "voice": job.voice,
        "speed": job.speed,
        "total_chunks": max(job.total_chunks, len(chunks)),
        "text": full_text,
        "chunks": [{"task_id": c.id, "chunk_index": c.chunk_index, "audio_path": c.audio_path, "status": c.status, "text": c.text} for c in chunks]
    }

class JobInitRequest(BaseModel):
    job_id: str
    engine: str
    voice: str
    speed: float
    total_chunks: int
    text: str

@router.post("/jobs/init")
def init_job(req: JobInitRequest, db: Session = Depends(get_db), current_user: User = Depends(get_current_active_user)):
    job = db.query(TTSJob).filter(TTSJob.id == req.job_id).first()
    if not job:
        job = TTSJob(
            id=req.job_id,
            user_id=current_user.id,
            engine=req.engine,
            voice=req.voice,
            speed=req.speed,
            total_chunks=req.total_chunks,
            text=req.text
        )
        db.add(job)
        db.commit()
    return {"message": "Job initialized successfully"}
