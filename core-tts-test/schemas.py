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
    AutoFormatRule(find=r"\u00D0", replace="\u0110"),
    AutoFormatRule(find=r"([a-zA-ZÀ-ỹ])\-([a-zA-ZÀ-ỹ])", replace=r"\1 \2"),
    AutoFormatRule(find=r"[⁰¹²³⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉]", replace=""),
    AutoFormatRule(find=r"[^a-zA-Z0-9 \n\t\r.,?!;:\-\"'()\[\]%/“”‘’À-ỹ]", replace="")
]

class InputPanelSpec(BaseModel):
    file_serve: bool = True
    closeable: bool = False
    find_mode: str = "expert"
    replace_tool: bool = True
    enable_chunk_box: bool = True
    max_chunk_size: int = 1000
    chunk_delimiters: List[str] = Field(default_factory=lambda: [r"(?<=\.\s*\n)", r"(?<=[.!?]\s+)"])
    auto_format: List[AutoFormatRule] = Field(default_factory=lambda: DEFAULT_AUTO_FORMAT_RULES)

class VoiceMetadataFieldSpec(BaseModel):
    key: str
    label: str
    type: str = "text"
    required: Optional[bool] = False
    placeholder: Optional[str] = None
    options: Optional[List[str]] = None

class ModelOptionSpec(BaseModel):
    notice_banner: Optional[NoticeBannerSpec] = None
    voice_type: Optional[str] = None
    speed_type: Optional[str] = None
    pitch_type: Optional[str] = None
    emotion_type: Optional[str] = None
    preset_voices: Optional[List[Dict[str, str]]] = None
    voice_metadata_schema: Optional[List[VoiceMetadataFieldSpec]] = None

class UISchemaSpec(BaseModel):
    ui_mode: str = Field(default_factory=lambda: os.getenv("UI_MODE", "beauty")) # "beauty" | "fast"
    input_panel: InputPanelSpec = Field(default_factory=InputPanelSpec)
    model_sort: List[str] = Field(default_factory=lambda: ["fast", "express", "zero_shot_clone", "multilingual", "emotion_v2", "standard", "clone"])
    option_panel: Dict[str, ModelOptionSpec] = Field(default_factory=lambda: {
        "fast": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="info",
                message="⚡ Cloud Fast: Ultra-fast simulated TTS responses."
            ),
            voice_type="radio",
            speed_type="slider",
            preset_voices=[
                {"id": "Mock Voice A (Female)", "name": "Mock Voice A (Female)", "gender": "female"},
                {"id": "Mock Voice B (Male)", "name": "Mock Voice B (Male)", "gender": "male"}
            ]
        ),
        "express": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="success",
                message="✅ Express Real-Time: Low-latency streaming test engine ready."
            ),
            voice_type="select",
            speed_type="slider"
        ),
        "zero_shot_clone": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="info",
                message="⚡ Instant Zero-Shot Clone: Supports temporary audio upload without saving voice profiles to account."
            ),
            voice_type="select",
            speed_type="slider"
        ),
        "multilingual": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="warning",
                message="⚠️ Multilingual: Cross-lingual synthesis with regional accent selection."
            ),
            voice_type="select",
            speed_type="slider"
        ),
        "emotion_v2": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="danger",
                message="🚨 Emotion & Style: Experimental model - dynamic pitch controls active."
            ),
            voice_type="select",
            speed_type="slider",
            pitch_type="slider",
            emotion_type="select"
        ),
        "standard": ModelOptionSpec(
            voice_type="select",
            speed_type="slider"
        ),
        "clone": ModelOptionSpec(
            voice_type="select",
            speed_type="slider",
            voice_metadata_schema=[
                VoiceMetadataFieldSpec(key="name", label="Voice Name", type="text", required=True, placeholder="e.g. Test Voice..."),
                VoiceMetadataFieldSpec(key="gender", label="Gender", type="select", options=["Male", "Female", "Other"]),
                VoiceMetadataFieldSpec(key="region", label="Accent / Region", type="select", options=["North American", "British", "Australian", "Other"]),
                VoiceMetadataFieldSpec(key="style", label="Style", type="select", options=["Expressive", "News / Broadcast", "Audiobook / Reading", "Dramatic", "Natural / Conversational", "Commercial", "Other"])
            ]
        )
    })

class UniversalManifest(BaseModel):
    engine_id: str = Field(default_factory=lambda: os.getenv("ENGINE_ID", "core-tts-test-v1"))
    engine_name: str = Field(default_factory=lambda: os.getenv("ENGINE_NAME", "Mock Test AI Engine"))
    version: str = Field(default_factory=lambda: os.getenv("ENGINE_VERSION", "1.0.0-mock"))
    provider: str = Field(default_factory=lambda: os.getenv("ENGINE_PROVIDER", "Universal AI Testbed"))
    supported_modes: List[EngineModeSpec] = Field(default_factory=lambda: [
        EngineModeSpec(id="standard", name="Mock Standard", description="Fast mock audio generator", supports_preset_voices=True, supports_cloning=False, supports_voice_saving=False, supports_streaming=True),
        EngineModeSpec(id="fast", name="Mock Fast", description="Instant mock audio generator", supports_preset_voices=True, supports_cloning=False, supports_voice_saving=False, supports_streaming=True),
        EngineModeSpec(id="express", name="Mock Express", description="Ultra-low latency streaming model", supports_preset_voices=True, supports_cloning=False, supports_voice_saving=False, supports_streaming=True),
        EngineModeSpec(id="zero_shot_clone", name="Mock Instant Zero-Shot Clone", description="Instant voice cloning from uploaded reference audio without saving to library", supports_preset_voices=False, supports_cloning=True, supports_voice_saving=False, supports_streaming=True),
        EngineModeSpec(id="multilingual", name="Mock Multilingual", description="Cross-lingual multi-accent voice engine", supports_preset_voices=True, supports_cloning=False, supports_voice_saving=False, supports_streaming=True),
        EngineModeSpec(id="emotion_v2", name="Mock Emotion & Style", description="Dynamic prosody & pitch control model", supports_preset_voices=True, supports_cloning=False, supports_voice_saving=False, supports_streaming=True),
        EngineModeSpec(id="clone", name="Mock Voice Cloning", description="Simulated speaker cloning", supports_preset_voices=True, supports_cloning=True, supports_voice_saving=True, supports_streaming=True)
    ])
    capabilities: EngineCapabilities = Field(default_factory=EngineCapabilities)
    constraints: EngineConstraints = Field(default_factory=EngineConstraints)
    audio_spec: AudioSpec = Field(default_factory=AudioSpec)
    ui_schema: Optional[UISchemaSpec] = Field(default_factory=UISchemaSpec)

CoreInfoResponse = UniversalManifest
