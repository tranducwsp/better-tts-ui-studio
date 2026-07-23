from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker, declarative_base
import os

# Lấy URL từ biến môi trường, mặc định là SQLite
DATABASE_URL = os.getenv("DATABASE_URL", "sqlite:///./tts_database.db")

# Nếu dùng SQLite thì cần check_same_thread=False
connect_args = {"check_same_thread": False} if DATABASE_URL.startswith("sqlite") else {}

engine = create_engine(DATABASE_URL, connect_args=connect_args)
SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)

Base = declarative_base()

def get_db():
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()

def register_job_and_chunk(user_id: str, job_id: str, engine: str, voice: str, speed: float, total_chunks: int, task_id: str, chunk_index: int, text: str):
    from .models import TTSJob, TTSChunk
    db = SessionLocal()
    try:
        job = db.query(TTSJob).filter(TTSJob.id == job_id).first()
        if not job:
            try:
                new_job = TTSJob(
                    id=job_id,
                    user_id=user_id,
                    engine=engine,
                    voice=voice,
                    speed=speed,
                    total_chunks=total_chunks
                )
                db.add(new_job)
                db.commit()
            except Exception:
                # Nếu có luồng khác đã insert job_id này rồi, ta bỏ qua lỗi
                db.rollback()
        
        chunk = TTSChunk(
            id=task_id,
            job_id=job_id,
            chunk_index=chunk_index,
            text=text,
            status="processing"
        )
        db.add(chunk)
        db.commit()
    finally:
        db.close()

def update_chunk_status(task_id: str, status: str, audio_path: str = None, error_msg: str = None):
    from .models import TTSChunk
    db = SessionLocal()
    try:
        chunk = db.query(TTSChunk).filter(TTSChunk.id == task_id).first()
        if chunk:
            chunk.status = status
            if audio_path:
                chunk.audio_path = audio_path
            if error_msg:
                chunk.error_msg = error_msg
            db.commit()
    finally:
        db.close()
