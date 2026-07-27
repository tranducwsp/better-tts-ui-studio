import json
import asyncio
from fastapi import APIRouter, HTTPException, Response, Depends
from fastapi.responses import StreamingResponse

from core.models import User
from api.auth import get_current_active_user
from core.state import tasks_db

# SSE stream is typically mounted directly in api/router.py to match /api/stream/tasks/{task_id}
# We will create a separate router for it or just keep it here and mount without prefix for stream.
# Let's remove the prefix in api/router.py for tasks and add it manually here.

router = APIRouter()

@router.get("/tasks/{task_id}")
def get_task_status(task_id: str, current_user: User = Depends(get_current_active_user)):
    if task_id not in tasks_db:
        raise HTTPException(status_code=404, detail="Không tìm thấy task")
    return {"status": tasks_db[task_id]["status"], "progress": tasks_db[task_id]["progress"]}

@router.post("/tasks/{task_id}/cancel")
def cancel_task(task_id: str, current_user: User = Depends(get_current_active_user)):
    if task_id in tasks_db:
        tasks_db[task_id]["cancel"] = True
        tasks_db[task_id]["status"] = "cancelled"
    return {"message": "Đã yêu cầu hủy"}

@router.get("/tasks/{task_id}/audio")
def get_task_audio(task_id: str, format: str = "wav", current_user: User = Depends(get_current_active_user)):
    if task_id not in tasks_db or tasks_db[task_id]["status"] != "done":
        raise HTTPException(status_code=404, detail="Audio chưa sẵn sàng")
    
    task = tasks_db[task_id]
    
    if format.lower() == "mp3" and "audio_mp3" in task:
        return Response(
            content=task["audio_mp3"], 
            media_type="audio/mpeg",
            headers={"Content-Disposition": 'attachment; filename="vieneu_tts_audio.mp3"'}
        )
            
    # Default trả về WAV
    return Response(
        content=task["audio"], 
        media_type="audio/wav",
        headers={"Content-Disposition": 'attachment; filename="vieneu_tts_audio.wav"'}
    )

@router.get("/stream/tasks/{task_id}")
async def stream_task_progress(task_id: str, current_user: User = Depends(get_current_active_user)):
    if task_id not in tasks_db:
        raise HTTPException(status_code=404, detail="Task not found")
        
    if "queue" not in tasks_db[task_id]:
        tasks_db[task_id]["queue"] = asyncio.Queue()
        
    queue = tasks_db[task_id]["queue"]
    
    async def event_generator():
        # Nếu task đã xong hoặc lỗi từ trước, báo luôn
        current_status = tasks_db[task_id]["status"]
        yield f"data: {json.dumps({'status': current_status, 'progress': tasks_db[task_id].get('progress', 0)})}\n\n"
        
        if current_status in ["done", "error", "cancelled"]:
            return
            
        while True:
            update = await queue.get()
            yield f"data: {json.dumps(update)}\n\n"
            if update["status"] in ["done", "error", "cancelled"]:
                break

    return StreamingResponse(event_generator(), media_type="text/event-stream")
