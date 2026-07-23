import io
import re
import uuid
import time
import gc
import asyncio
import os
import edge_tts
from fastapi import APIRouter, HTTPException, BackgroundTasks, Depends

from core.models import User
from api.auth import get_current_active_user
from core.schemas import FastTTSRequest
from core.state import FAST_VOICES, tasks_db, cleanup_tasks_db
from core.database import register_job_and_chunk, update_chunk_status

router = APIRouter()

@router.get("/voices")
def get_fast_voices(current_user: User = Depends(get_current_active_user)):
    return list(FAST_VOICES.keys())

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
            voice_code = FAST_VOICES.get(req.voice, "vi-VN-HoaiMyNeural")
            rate_percent = int((req.speed - 1.0) * 100)
            rate_str = f"{rate_percent:+d}%" if rate_percent != 0 else None

            def split_text(txt, target_chars=800, max_chars=2000):
                txt = txt.replace('\r\n', '\n')
                txt = re.sub(r'\n{3,}', '\n\n', txt)
                txt = txt.replace('\u00D0', '\u0110')
                txt = re.sub(r'([^\W\d_])\-([^\W\d_])', r'\1 \2', txt)
                txt = re.sub(r'[⁰¹²³⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉]', '', txt)
                txt = re.sub(r'[^a-zA-Z0-9\s.,?!;:"\'()\[\]%\-/“”‘’À-ỹ]', '', txt)
                txt = txt.strip()
                if len(txt) <= target_chars:
                    return [txt]
                paragraphs = re.split(r'(?<=\.\n)', txt)
                chunks = []
                curr = ""
                for p in paragraphs:
                    clean_p = p.strip()
                    if not clean_p: continue
                    if len(clean_p) > max_chars:
                        sentences = re.split(r'(?<=[.!?])\s+', clean_p)
                        for s in sentences:
                            clean_s = s.strip()
                            if not clean_s: continue
                            if len(curr) + len(clean_s) + 1 <= target_chars:
                                curr += (" " if curr else "") + clean_s
                            else:
                                if curr: chunks.append(curr)
                                curr = clean_s
                    else:
                        if len(curr) + len(clean_p) + 1 <= target_chars:
                            curr += ("\n" if curr else "") + clean_p
                        else:
                            if curr: chunks.append(curr)
                            curr = clean_p
                if curr: chunks.append(curr)
                return chunks if chunks else [txt]

            text_chunks = split_text(req.text)
            
            async def generate_chunk(text_segment, idx):
                if not text_segment.strip() or not any(c.isalnum() for c in text_segment):
                    return idx, b""
                if rate_str:
                    communicate = edge_tts.Communicate(text_segment, voice_code, rate=rate_str)
                else:
                    communicate = edge_tts.Communicate(text_segment, voice_code)
                chunk_audio = b""
                async for chunk in communicate.stream():
                    if tasks_db[task_id].get("cancel"):
                        return idx, b""
                    if chunk["type"] == "audio":
                        chunk_audio += chunk["data"]
                return idx, chunk_audio

            async def run_edge_parallel():
                tasks = []
                for i, segment in enumerate(text_chunks):
                    tasks.append(generate_chunk(segment, i))
                
                results = await asyncio.gather(*tasks)
                results.sort(key=lambda x: x[0])
                
                final_audio = b"".join([res[1] for res in results])
                return final_audio

            mp3_bytes = asyncio.run(run_edge_parallel())

            # Save to MP3 directly
            tasks_db[task_id]["audio_mp3"] = mp3_bytes
            tasks_db[task_id]["audio"] = mp3_bytes # Fast mode only MP3
            
            # Save file to disk for history
            os.makedirs("storage/temp", exist_ok=True)
            file_path = f"storage/temp/{task_id}.mp3"
            with open(file_path, "wb") as f:
                f.write(mp3_bytes)

            if job_id:
                update_chunk_status(task_id, "done", file_path)

            tasks_db[task_id]["progress"] = 100
            tasks_db[task_id]["status"] = "done"
            if "queue" in tasks_db[task_id]:
                loop.call_soon_threadsafe(tasks_db[task_id]["queue"].put_nowait, {"status": "done"})
        except Exception as e:
            import traceback
            traceback.print_exc()
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
