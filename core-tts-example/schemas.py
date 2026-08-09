import os
from pydantic import BaseModel, Field
from typing import Optional, Dict, Any, List

class SynthesizeRequest(BaseModel):
    """Payload the platform sends to /synthesize.

    Only fields the platform actually transmits are listed here. An AI engineer
    may add more (e.g. output_format, ref_voice_id) for internal use — the
    platform ignores unknown fields in the request it sends.
    """
    text: str
    voice_id: Optional[str] = None
    speed: float = 1.0
    engine: Optional[str] = None
    # pitch and emotion are only sent when the mode's capabilities declare
    # supports_pitch / supports_emotion. When absent, the value is None —
    # distinct from 0.0, which means "explicitly set to 0".
    pitch: Optional[float] = None
    emotion: Optional[str] = None

class VoiceInfo(BaseModel):
    """A preset voice the engine ships with.

    `modes` names the modes this voice works in, and is required — same as the real engine's
    schema. It stayed absent here after `modes` was tightened from optional to required, so
    this mock answered /voices with every voice for every mode while the real engine filtered.
    A mock that is laxer than the contract cannot catch the bug it exists to catch: the UI
    offering a voice its mode cannot use looked fine in development and only broke against
    the real engine.

    (The platform does NOT read the `modes` field from the response. It sends
    `?model_id=<mode>` as a query parameter and relies on the engine to filter. The field
    is kept here because the example engine uses it for its own filtering logic.)
    """
    id: str
    name: str
    descriptions: List[str] = Field(default_factory=list)
    modes: List[str]

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

class EngineModeSpec(BaseModel):
    """One processing mode. Capabilities stated here override the engine-wide set."""
    id: str
    name: str
    description: str = ""
    capabilities: EngineCapabilities = Field(default_factory=lambda: EngineCapabilities())
    # Chỉ khai những trường khác với audio_spec toàn Engine.
    audio_spec: AudioSpec = Field(default_factory=lambda: AudioSpec())

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
    # Available emotion labels for modes that declare supports_emotion.
    supported_emotions: List[str] = Field(default_factory=lambda: ["neutral", "happy", "sad", "angry", "excited"])
    chunking: ChunkingSpec = Field(default_factory=ChunkingSpec)

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
    model_sort: List[str] = Field(default_factory=lambda: ["fast", "standard", "zero_shot_clone"])
    option_panel: Dict[str, ModelOptionSpec] = Field(default_factory=lambda: {
        "fast": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="warning",
                message="⚡ Chế độ Siêu Nhanh: Tổng hợp tốc độ cao, phù hợp nghe thử nhanh."
            ),
            voice_type="radio",
            speed_type="slider",
            preset_voices=[
                {"id": "hoai_my", "name": "Hoài Mỹ", "gender": "female"},
                {"id": "nam_minh", "name": "Nam Minh", "gender": "male"}
            ]
        ),
        "standard": ModelOptionSpec(
            voice_type="select",
            speed_type="slider"
        ),
        "zero_shot_clone": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="info",
                message="⚡ Clone Giọng: Tải âm thanh tham chiếu để sao chép giọng nói, lưu giọng vào tài khoản."
            ),
            voice_type="select",
            speed_type="slider",
            voice_metadata_schema=[
                VoiceMetadataFieldSpec(key="name", label="Tên giọng", type="text", required=True, placeholder="VD: Giọng của tôi..."),
                VoiceMetadataFieldSpec(key="gender", label="Giới tính", type="select", options=["Nam", "Nữ", "Khác"]),
                VoiceMetadataFieldSpec(key="region", label="Vùng miền", type="select", options=["Miền Bắc", "Miền Trung", "Miền Nam", "Khác"]),
                VoiceMetadataFieldSpec(key="style", label="Phong cách", type="select", options=["Tự nhiên", "Bản tin", "Sách nói", "Diễn xuất", "Khác"])
            ]
        )
    })

class UniversalManifest(BaseModel):
    engine_id: str = Field(default_factory=lambda: os.getenv("ENGINE_ID", "core-tts-example-v1"))
    engine_name: str = Field(default_factory=lambda: os.getenv("ENGINE_NAME", "Example TTS Engine"))
    version: str = Field(default_factory=lambda: os.getenv("ENGINE_VERSION", "1.0.0"))
    provider: str = Field(default_factory=lambda: os.getenv("ENGINE_PROVIDER", "Example Provider"))
    supported_modes: List[EngineModeSpec] = Field(default_factory=lambda: [
        EngineModeSpec(id="fast", name="Siêu Nhanh", description="Tổng hợp giọng nói tốc độ cao"),
        EngineModeSpec(id="standard", name="TTS Cơ Bản", description="Tổng hợp giọng nói tiêu chuẩn"),
        EngineModeSpec(id="zero_shot_clone", name="Giọng Clone",
                       description="Sao chép giọng nói từ âm thanh tham chiếu",
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

