from pydantic import BaseModel
from typing import Optional, Dict, Any, List

class SynthesizeRequest(BaseModel):
    text: str
    voice_id: Optional[str] = "minh_duc"
    voice: Optional[str] = None
    engine: Optional[str] = "standard" # "standard" | "fast" | "clone"
    speed: float = 1.0
    pitch: float = 0.0
    output_format: str = "wav"
    ref_voice_id: Optional[str] = None
    task_id: Optional[str] = None

class TaskStatusResponse(BaseModel):
    task_id: str
    status: str # "pending" | "processing" | "done" | "error" | "cancelled"
    progress: int = 0
    audio_url: Optional[str] = None
    error: Optional[str] = None

class VoiceInfo(BaseModel):
    id: str
    name: str
    type: str = "standard"
    language: str = "vi-VN"
    gender: Optional[str] = None
    region: Optional[str] = None
    style: Optional[str] = None

class CoreInfoResponse(BaseModel):
    engine_name: str = "VieNeu-TTS-Core"
    version: str = "1.0.0"
    capabilities: Dict[str, bool] = {
        "supports_cloning": True,
        "supports_speed": True,
        "supports_pitch": True
    }
    audio_formats: List[str] = ["wav", "mp3"]
    sample_rates: List[int] = [22050, 24000, 44100]
