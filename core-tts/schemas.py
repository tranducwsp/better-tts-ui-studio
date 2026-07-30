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
    supports_preset_voices: bool = True
    supports_cloning: bool = False
    supports_voice_saving: bool = False
    supports_streaming: bool = True

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

DEFAULT_AUTO_FORMAT_RULES = [
    AutoFormatRule(find=r"\r\n", replace="\n"),
    AutoFormatRule(find=r"\n{3,}", replace="\n\n"),
    AutoFormatRule(find=r"\u00D0", replace="\u0110"),  # Ð -> Đ (Eth to Vietnamese Đ)
    AutoFormatRule(find=r"([a-zA-ZÀ-ỹ])\-([a-zA-ZÀ-ỹ])", replace=r"\1 \2"), # Un-hyphenate words
    AutoFormatRule(find=r"[⁰¹²³⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉]", replace=""), # Remove superscript/subscript footnote numbers
    AutoFormatRule(find=r"[^a-zA-Z0-9 \n\t\r.,?!;:\-\"'()\[\]%/“”‘’À-ỹ]", replace="") # Clean non-Vietnamese strange characters
]

class InputPanelSpec(BaseModel):
    file_serve: bool = True
    closeable: bool = False
    find_mode: str = "expert" # "express" (tìm kiếm chuỗi đơn giản) | "expert" (cho phép bật/tắt công cụ Regex)
    replace_tool: bool = True
    enable_chunk_box: bool = True
    max_chunk_size: Optional[int] = 1000
    chunk_delimiters: Optional[List[str]] = Field(default_factory=lambda: [r"(?<=\.\s*\n)", r"(?<=[.!?]\s+)"])
    auto_format: List[AutoFormatRule] = Field(default_factory=lambda: DEFAULT_AUTO_FORMAT_RULES)

class VoiceMetadataFieldSpec(BaseModel):
    key: str
    label: str
    type: str = "text" # "text", "select"
    required: Optional[bool] = False
    placeholder: Optional[str] = None
    options: Optional[List[str]] = None

class ModelOptionSpec(BaseModel):
    notice_banner: Optional[NoticeBannerSpec] = None
    voice_type: Optional[str] = None # "select", "radio"
    speed_type: Optional[str] = None # "slider", "number", "stepped"
    pitch_type: Optional[str] = None
    emotion_type: Optional[str] = None
    preset_voices: Optional[List[Dict[str, str]]] = None
    voice_metadata_schema: Optional[List[VoiceMetadataFieldSpec]] = None

class UISchemaSpec(BaseModel):
    ui_mode: str = Field(default_factory=lambda: os.getenv("UI_MODE", "beauty")) # "beauty" | "fast"
    input_panel: InputPanelSpec = Field(default_factory=InputPanelSpec)
    model_sort: List[str] = Field(default_factory=lambda: ["fast", "standard", "clone"])
    option_panel: Dict[str, ModelOptionSpec] = Field(default_factory=lambda: {
        "fast": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="warning",
                message="Note: This cloud voice model processes data externally. Avoid sending confidential information."
            ),
            voice_type="radio",
            speed_type="slider",
            preset_voices=[
                {"id": "Voice A (Female)", "name": "Voice A (Female)", "gender": "female"},
                {"id": "Voice B (Male)", "name": "Voice B (Male)", "gender": "male"}
            ]
        ),
        "standard": ModelOptionSpec(
            voice_type="select",
            speed_type="slider"
        ),
        "clone": ModelOptionSpec(
            voice_type="select",
            speed_type="slider",
            voice_metadata_schema=[
                VoiceMetadataFieldSpec(key="name", label="Voice Name", type="text", required=True, placeholder="e.g. Male Narrator..."),
                VoiceMetadataFieldSpec(key="gender", label="Gender", type="select", options=["Male", "Female", "Other"]),
                VoiceMetadataFieldSpec(key="region", label="Accent / Region", type="select", options=["North American", "British", "Australian", "Other"]),
                VoiceMetadataFieldSpec(key="style", label="Style", type="select", options=["Expressive", "News / Broadcast", "Audiobook / Reading", "Dramatic", "Natural / Conversational", "Commercial", "Other"])
            ]
        )
    })

class UniversalManifest(BaseModel):
    engine_id: str = Field(default_factory=lambda: os.getenv("ENGINE_ID", "core-engine-v1"))
    engine_name: str = Field(default_factory=lambda: os.getenv("ENGINE_NAME", "Universal Core AI Engine"))
    version: str = Field(default_factory=lambda: os.getenv("ENGINE_VERSION", "1.0.0"))
    provider: str = Field(default_factory=lambda: os.getenv("ENGINE_PROVIDER", "Universal AI Platform"))
    supported_modes: List[EngineModeSpec] = Field(default_factory=lambda: [
        EngineModeSpec(id="standard", name="Standard Neural", description="High fidelity neural voice inference", supports_preset_voices=True, supports_cloning=False, supports_voice_saving=False, supports_streaming=True),
        EngineModeSpec(id="fast", name="Fast Streaming", description="Low latency streaming TTS", supports_preset_voices=True, supports_cloning=False, supports_voice_saving=False, supports_streaming=True),
        EngineModeSpec(id="clone", name="Voice Cloning", description="Reference audio speaker cloning", supports_preset_voices=True, supports_cloning=True, supports_voice_saving=True, supports_streaming=True)
    ])
    capabilities: EngineCapabilities = Field(default_factory=EngineCapabilities)
    constraints: EngineConstraints = Field(default_factory=EngineConstraints)
    audio_spec: AudioSpec = Field(default_factory=AudioSpec)
    ui_schema: Optional[UISchemaSpec] = Field(default_factory=UISchemaSpec)

# Legacy compatibility alias
CoreInfoResponse = UniversalManifest
