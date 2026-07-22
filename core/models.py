from sqlalchemy import Column, Integer, String, Boolean, DateTime, Float, ForeignKey
from sqlalchemy.sql import func
from sqlalchemy.orm import relationship
from .database import Base

class User(Base):
    __tablename__ = "users"

    id = Column(String, primary_key=True, index=True)
    username = Column(String, unique=True, index=True, nullable=False)
    password_hash = Column(String, nullable=False)
    role = Column(String, default="user") # 'user' or 'admin'
    is_approved = Column(Boolean, default=False)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    
    voices = relationship("UserVoice", back_populates="user", cascade="all, delete-orphan")

class UserVoice(Base):
    __tablename__ = "user_voices"

    id = Column(String, primary_key=True, index=True)
    user_id = Column(String, ForeignKey("users.id"))
    name = Column(String, nullable=False)
    gender = Column(String, nullable=True)
    region = Column(String, nullable=True)
    style = Column(String, nullable=True)
    file_path = Column(String, nullable=False)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    
    user = relationship("User", back_populates="voices")

class TTSJob(Base):
    __tablename__ = "tts_jobs"

    id = Column(String, primary_key=True, index=True) # job_id from frontend
    user_id = Column(String, ForeignKey("users.id"))
    engine = Column(String) # standard, clone, fast
    voice = Column(String) # voice name or clone id
    speed = Column(Float)
    total_chunks = Column(Integer)
    text = Column(String)
    created_at = Column(DateTime(timezone=True), server_default=func.now())
    
    user = relationship("User")
    chunks = relationship("TTSChunk", back_populates="job", cascade="all, delete-orphan")

class TTSChunk(Base):
    __tablename__ = "tts_chunks"

    id = Column(String, primary_key=True, index=True) # task_id
    job_id = Column(String, ForeignKey("tts_jobs.id"))
    chunk_index = Column(Integer)
    text = Column(String)
    audio_path = Column(String, nullable=True)
    status = Column(String, default="pending") # pending, processing, done, error
    error_msg = Column(String, nullable=True)
    
    job = relationship("TTSJob", back_populates="chunks")
