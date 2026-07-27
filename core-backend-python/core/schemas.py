from pydantic import BaseModel, model_validator
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
    clone_id: Optional[str] = None
    voice: Optional[str] = None
    speed: float = 1.0
    job_id: Optional[str] = None
    chunk_index: Optional[int] = None
    total_chunks: Optional[int] = None
    task_id: Optional[str] = None

    @model_validator(mode='after')
    def check_clone_id(self):
        if not self.clone_id and self.voice:
            self.clone_id = self.voice
        if not self.clone_id:
            raise ValueError("Cần truyền clone_id hoặc voice")
        return self

class FastTTSRequest(BaseModel):
    text: str
    voice: str = "Hoài Mỹ (Nữ)"
    speed: float = 1.0
    job_id: Optional[str] = None
    chunk_index: Optional[int] = None
    total_chunks: Optional[int] = None
    task_id: Optional[str] = None
