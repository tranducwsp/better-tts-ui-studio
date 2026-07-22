from pydantic import BaseModel
from typing import Optional

class TTSRequest(BaseModel):
    text: str
    voice: str = "Phạm Tuyên"
    speed: float = 1.0
    job_id: Optional[str] = None
    chunk_index: Optional[int] = None
    total_chunks: Optional[int] = None
    task_id: Optional[str] = None

class CloneSynthesizeRequest(BaseModel):
    text: str
    clone_id: str
    speed: float = 1.0
    job_id: Optional[str] = None
    chunk_index: Optional[int] = None
    total_chunks: Optional[int] = None
    task_id: Optional[str] = None

class FastTTSRequest(BaseModel):
    text: str
    voice: str = "Hoài Mỹ (Nữ - Review Phim)"
    speed: float = 1.0
    job_id: Optional[str] = None
    chunk_index: Optional[int] = None
    total_chunks: Optional[int] = None
    task_id: Optional[str] = None
