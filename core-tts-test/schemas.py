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

class EngineCapabilities(BaseModel):
    """What an engine (or one of its modes) can do.

    Every field is Optional and defaults to None, meaning "not stated". A mode's own value
    wins if it states one, otherwise the engine-wide value applies. None is therefore
    distinct from False: False means "explicitly cannot", None means "inherit".
    """
    supports_preset_voices: Optional[bool] = None
    supports_cloning: Optional[bool] = None
    supports_voice_saving: Optional[bool] = None
    supports_streaming: Optional[bool] = None
    supports_speed: Optional[bool] = None
    supports_pitch: Optional[bool] = None
    supports_emotion: Optional[bool] = None
    supports_ssml: Optional[bool] = None

class EngineModeSpec(BaseModel):
    """One processing mode. Capabilities stated here override the engine-wide set."""
    id: str
    name: str
    description: str = ""
    capabilities: EngineCapabilities = Field(default_factory=lambda: EngineCapabilities())

class RangeConstraint(BaseModel):
    min: float = 0.5
    max: float = 2.0
    default: float = 1.0
    step: float = 0.1

class ChunkingSpec(BaseModel):
    """How the platform must divide text longer than max_text_length.

    Lives under constraints, not ui_schema: it decides what is transmitted, not how
    anything looks. `delimiters` is an ordered list of cut points, most preferred first;
    the platform tries each on any fragment still too long, then hard-splits the rest.
    Patterns are used with split(), so zero-width lookbehinds are the expected shape.
    """
    max_chunk_size: Optional[int] = 1000
    delimiters: List[str] = Field(default_factory=lambda: [r"(?<=\.\s*\n)", r"(?<=[.!?]\s+)"])

class EngineConstraints(BaseModel):
    max_text_length: int = 3000
    speed_range: RangeConstraint = Field(default_factory=RangeConstraint)
    pitch_range: RangeConstraint = Field(default_factory=lambda: RangeConstraint(min=-10.0, max=10.0, default=0.0, step=0.5))
    # Non-empty because one mode (emotion_v2) declares supports_emotion; a mode claiming
    # emotion support with no vocabulary to choose from would be a contradiction.
    supported_emotions: List[str] = Field(default_factory=lambda: ["neutral", "happy", "sad", "angry", "excited"])
    chunking: ChunkingSpec = Field(default_factory=ChunkingSpec)

class AudioSpec(BaseModel):
    """Định dạng Engine xuất ra, và ràng buộc cho âm thanh tham chiếu nhận vào.

    Xuất hiện hai tầng — toàn Engine và theo Mode — nên mọi trường Optional: None nghĩa là
    kế thừa tầng trên.
    """
    supported_formats: Optional[List[str]] = None
    supported_sample_rates: Optional[List[int]] = None
    default_format: Optional[str] = None
    default_sample_rate: Optional[int] = None

    # Định dạng đọc được cho âm thanh tham chiếu. Engine mock này nhận cả MP3 để chứng minh
    # rằng nền tảng không giả định WAV — không suy được từ supported_formats ở trên.
    reference_audio_formats: Optional[List[str]] = None
    reference_audio_seconds: Optional[float] = None

    # Hai trần cho hai thời điểm: tệp thô kéo vào, và clip sau khi cắt.
    max_upload_bytes: Optional[int] = None
    max_reference_bytes: Optional[int] = None

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
    auto_format: List[AutoFormatRule] = Field(default_factory=lambda: DEFAULT_AUTO_FORMAT_RULES)

class VoiceMetadataFieldSpec(BaseModel):
    key: str
    label: str
    type: str = "text"
    required: Optional[bool] = False
    placeholder: Optional[str] = None
    options: Optional[List[str]] = None

class PresetVoiceSpec(BaseModel):
    """A voice the engine ships with, listed in the manifest instead of via /voices."""
    id: str
    name: str
    gender: Optional[str] = None
    descriptions: List[str] = Field(default_factory=list)
    sample_url: Optional[str] = None

class ModelOptionSpec(BaseModel):
    """Presentation only: how a control is drawn, never whether it exists.

    Whether a control appears is decided by capabilities; these fields pick the widget for
    one already known to be supported.
    """
    notice_banner: Optional[NoticeBannerSpec] = None
    voice_type: Optional[str] = None
    speed_type: Optional[str] = None
    pitch_type: Optional[str] = None
    emotion_type: Optional[str] = None
    preset_voices: Optional[List[PresetVoiceSpec]] = None
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
                message="⚡ Instant Zero-Shot Clone: Supports reference audio upload & saving voice profiles to account."
            ),
            voice_type="select",
            speed_type="slider",
            voice_metadata_schema=[
                VoiceMetadataFieldSpec(key="name", label="Voice Name", type="text", required=True, placeholder="e.g. My Cloned Voice..."),
                VoiceMetadataFieldSpec(key="gender", label="Gender", type="select", options=["Male", "Female", "Other"]),
                VoiceMetadataFieldSpec(key="region", label="Accent / Region", type="select", options=["North American", "British", "Australian", "Other"]),
                VoiceMetadataFieldSpec(key="style", label="Style", type="select", options=["Expressive", "News / Broadcast", "Audiobook / Reading", "Dramatic", "Natural / Conversational", "Commercial", "Other"])
            ]
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
        # Only what differs from the engine-wide set below is stated here; anything left
        # unset is inherited. emotion_v2 is the interesting one: it is the sole mode that
        # can do pitch and emotion, which the engine-wide defaults switch off.
        EngineModeSpec(id="standard", name="Mock Standard", description="Fast mock audio generator"),
        EngineModeSpec(id="fast", name="Mock Fast", description="Instant mock audio generator"),
        EngineModeSpec(id="express", name="Mock Express", description="Ultra-low latency streaming model"),
        EngineModeSpec(id="zero_shot_clone", name="Mock Instant Zero-Shot Clone",
                       description="Instant voice cloning from uploaded reference audio with voice saving support",
                       capabilities=EngineCapabilities(supports_cloning=True, supports_voice_saving=True)),
        EngineModeSpec(id="multilingual", name="Mock Multilingual", description="Cross-lingual multi-accent voice engine"),
        EngineModeSpec(id="emotion_v2", name="Mock Emotion & Style",
                       description="Dynamic prosody & pitch control model",
                       capabilities=EngineCapabilities(supports_pitch=True, supports_emotion=True)),
        EngineModeSpec(id="clone", name="Mock Voice Cloning", description="Simulated speaker cloning",
                       capabilities=EngineCapabilities(supports_cloning=True, supports_voice_saving=True)),
    ])
    # Engine-wide defaults; modes that state nothing inherit these.
    capabilities: EngineCapabilities = Field(default_factory=lambda: EngineCapabilities(
        supports_preset_voices=True, supports_cloning=False, supports_voice_saving=False,
        supports_streaming=True, supports_speed=True,
        supports_pitch=False, supports_emotion=False, supports_ssml=False,
    ))
    constraints: EngineConstraints = Field(default_factory=EngineConstraints)
    audio_spec: AudioSpec = Field(default_factory=lambda: AudioSpec(
        supported_formats=["wav", "mp3"],
        supported_sample_rates=[16000, 22050, 24000, 44100],
        default_format="wav",
        default_sample_rate=24000,
        reference_audio_formats=["wav", "mp3", "flac"],
        reference_audio_seconds=3.0,
        max_upload_bytes=200 * 1024 * 1024,
        max_reference_bytes=5 * 1024 * 1024,
    ))
    ui_schema: Optional[UISchemaSpec] = Field(default_factory=UISchemaSpec)

