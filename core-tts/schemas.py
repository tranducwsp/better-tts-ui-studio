import os
from pydantic import BaseModel, Field
from typing import Optional, Dict, Any, List

class SynthesizeRequest(BaseModel):
    text: str
    voice_id: Optional[str] = None
    voice: Optional[str] = None
    engine: Optional[str] = None # "standard" | "fast" | "clone"
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
    descriptions: List[str] = Field(default_factory=list)

class EngineModeSpec(BaseModel):
    id: str
    name: str
    description: str = ""

class RangeConstraint(BaseModel):
    min: float = 0.5
    max: float = 2.0
    default: float = 1.0
    step: float = 0.1

class EngineConstraints(BaseModel):
    max_text_length: int = 3000
    speed_range: RangeConstraint = Field(default_factory=RangeConstraint)
    pitch_range: RangeConstraint = Field(default_factory=lambda: RangeConstraint(min=-10.0, max=10.0, default=0.0, step=0.5))
    supported_emotions: List[str] = Field(default_factory=list)

class AudioSpec(BaseModel):
    supported_formats: List[str] = Field(default_factory=lambda: ["wav", "mp3"])
    supported_sample_rates: List[int] = Field(default_factory=lambda: [16000, 22050, 24000, 44100])
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

class AutoFormatRule(BaseModel):
    find: str
    replace: str

class NoticeBannerSpec(BaseModel):
    level: str = "warning"
    message: str

class InputPanelSpec(BaseModel):
    file_serve: bool = True
    closeable: bool = False
    find_mode: str = "expert" # "express" (tìm kiếm chuỗi đơn giản) | "expert" (cho phép bật/tắt công cụ Regex)
    replace_tool: bool = True
    enable_chunk_box: bool = True
    auto_format: List[AutoFormatRule] = Field(default_factory=list)

class ModelOptionSpec(BaseModel):
    notice_banner: Optional[NoticeBannerSpec] = None
    voice_type: Optional[str] = None # "select", "radio"
    speed_type: Optional[str] = None # "slider", "number", "stepped"
    pitch_type: Optional[str] = None
    emotion_type: Optional[str] = None
    preset_voices: Optional[List[Dict[str, str]]] = None

class UISchemaSpec(BaseModel):
    input_panel: InputPanelSpec = Field(default_factory=InputPanelSpec)
    model_sort: List[str] = Field(default_factory=lambda: ["fast", "standard", "clone"])
    option_panel: Dict[str, ModelOptionSpec] = Field(default_factory=lambda: {
        "fast": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="warning",
                message="Lưu ý: Giọng đọc này kết nối qua cloud. Đối với dữ liệu cần bảo mật thì không nên dùng!"
            ),
            voice_type="radio",
            speed_type="slider",
            preset_voices=[
                {"id": "Hoài Mỹ (Nữ)", "name": "Hoài Mỹ (Nữ)", "gender": "female"},
                {"id": "Nam Minh (Nam)", "name": "Nam Minh (Nam)", "gender": "male"}
            ]
        ),
        "standard": ModelOptionSpec(
            voice_type="select",
            speed_type="slider"
        ),
        "clone": ModelOptionSpec(
            voice_type="select",
            speed_type="slider"
        )
    })

class UniversalManifest(BaseModel):
    engine_id: str = Field(default_factory=lambda: os.getenv("ENGINE_ID", "core-engine-v1"))
    engine_name: str = Field(default_factory=lambda: os.getenv("ENGINE_NAME", "Universal Core AI Engine"))
    version: str = Field(default_factory=lambda: os.getenv("ENGINE_VERSION", "1.0.0"))
    provider: str = Field(default_factory=lambda: os.getenv("ENGINE_PROVIDER", "Universal AI Platform"))
    supported_modes: List[EngineModeSpec] = Field(default_factory=lambda: [
        EngineModeSpec(id="standard", name="Standard Neural Engine", description="High fidelity neural voice inference"),
        EngineModeSpec(id="fast", name="Fast Streaming Engine", description="Low latency streaming TTS"),
        EngineModeSpec(id="clone", name="Voice Cloning Engine", description="Reference audio speaker cloning")
    ])
    capabilities: EngineCapabilities = Field(default_factory=EngineCapabilities)
    constraints: EngineConstraints = Field(default_factory=EngineConstraints)
    audio_spec: AudioSpec = Field(default_factory=AudioSpec)
    ui_schema: Optional[UISchemaSpec] = Field(default_factory=UISchemaSpec)

# Legacy compatibility alias
CoreInfoResponse = UniversalManifest
