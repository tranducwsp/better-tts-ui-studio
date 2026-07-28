from pydantic import BaseModel, Field
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

class RangeConstraint(BaseModel):
    min: float = 0.5
    max: float = 2.0
    default: float = 1.0
    step: float = 0.1

class EngineConstraints(BaseModel):
    max_text_length: int = 3000
    speed_range: RangeConstraint = Field(default_factory=RangeConstraint)
    pitch_range: RangeConstraint = Field(default_factory=lambda: RangeConstraint(min=-10.0, max=10.0, default=0.0, step=0.5))
    supported_emotions: List[str] = []

class AudioSpec(BaseModel):
    supported_formats: List[str] = ["wav", "mp3"]
    supported_sample_rates: List[int] = [16000, 22050, 24000, 44100]
    default_format: str = "wav"
    default_sample_rate: int = 24000

class EngineCapabilities(BaseModel):
    supports_preset_voices: bool = True
    supports_cloning: bool = True
    supports_streaming: bool = True
    supports_speed: bool = True
    supports_pitch: bool = False
    supports_emotion: bool = False
    supports_ssml: bool = False

class UniversalManifest(BaseModel):
    engine_id: str = "vieneu-v3turbo"
    engine_name: str = "VieNeu V3 Turbo Core Engine"
    version: str = "1.0.0"
    provider: str = "VieNeu Labs"
    capabilities: EngineCapabilities = Field(default_factory=EngineCapabilities)
    constraints: EngineConstraints = Field(default_factory=EngineConstraints)
    audio_spec: AudioSpec = Field(default_factory=AudioSpec)

# Legacy compatibility alias
CoreInfoResponse = UniversalManifest
