# 🛠️ AI Engineer Integration Guide: Pluggable Core TTS Manifest

Welcome to **AI Voice Studio**! This project is architected to be 100% engine-agnostic and schema-driven. As an AI Engineer, you do **NOT** need to edit frontend Svelte code or Go control-plane code to integrate your custom Speech Synthesis models (e.g., XTTS, GPT-SoVITS, VITS, Fish-Speech, Piper, Kokoro).

You only need to define your model's capabilities and UI specifications in Python (`schemas.py` or `manifest.json`).

---

## 📑 Table of Contents
1. [Core Architecture Overview](#1-core-architecture-overview)
2. [Step-by-Step Engine Integration](#2-step-by-step-engine-integration)
3. [Configuring UI Controls (`ui_schema`)](#3-configuring-ui-controls-ui_schema)
4. [Custom Voice Metadata Schema (`voice_metadata_schema`)](#4-custom-voice-metadata-schema-voice_metadata_schema)
5. [Text Normalization & Cleaning Rules (`auto_format`)](#5-text-normalization--cleaning-rules-auto_format)

---

## 1. Core Architecture Overview

```
┌───────────────────────────────┐
│     Frontend (Svelte 5)       │
└───────────────┬───────────────┘
                │ Dynamic UI built from Manifest
┌───────────────▼───────────────┐
│  Backend Gateway (Go)         │
└───────────────┬───────────────┘
                │ Proxy / REST / gRPC
┌───────────────▼───────────────┐
│  Engine (Python, external)    │ ◄── [YOUR CUSTOM AI MODEL HERE]
└───────────────────────────────┘
```

The Frontend & Go Backend dynamically request `/info` from your Python Engine service at runtime. They automatically:
- Render tabs for each mode defined in `supported_modes`.
- Render input sliders/dropdowns based on `capabilities` and `constraints`.
- Build the "Create Custom Voice" modal using `voice_metadata_schema`.
- Apply text cleaning filters using `auto_format` regex rules.

---

> A captured `GET /api/info` from the bundled engine lives in
> [`examples/manifest-engine.json`](./examples/manifest-engine.json) — quicker to adapt
> than building one from the schema.

## 2. Step-by-Step Engine Integration

### Step 1: Clone & Navigate to Core Engine
The compute microservice resides in your engine repo (e.g. `/engine/`).

### Step 2: Define your Model Schema in `engine/schemas.py`
In `schemas.py`, update `EngineManifestSpec`:

```python
class EngineManifestSpec(BaseModel):
    engine_id: str = "my-custom-tts"
    engine_name: str = "My Custom Voice Engine"
    version: str = "1.0.0"
    provider: str = "My AI Lab"
    
    # Define supported modes (e.g. standard, clone, fast)
    supported_modes: List[EngineModeSpec] = Field(default_factory=lambda: [
        EngineModeSpec(
            id="standard",
            name="Standard Neural Engine",
            description="High fidelity voice synthesis",
            supports_preset_voices=True
        ),
        EngineModeSpec(
            id="clone",
            name="Voice Cloning Engine",
            description="Zero-shot reference audio cloning",
            supports_cloning=True,
            supports_voice_saving=True
        )
    ])
```

### Step 3: Implement your Synthesis Logic in `engine/engine.py`
Override the `synthesize()` function to load your weights (e.g., via PyTorch or ONNX runtime):

```python
def synthesize(self, request: SynthesizeRequest) -> bytes:
    # Your model inference logic here:
    # audio_tensor = self.model.infer(request.text, request.voice_id, speed=request.speed)
    # return convert_tensor_to_wav_bytes(audio_tensor)
    pass
```

---

## 3. Configuring UI Controls (`ui_schema`)

You can customize how the UI displays for your model options in `UISchemaSpec`:

```python
class UISchemaSpec(BaseModel):
    input_panel: InputPanelSpec = Field(default_factory=InputPanelSpec)
    model_sort: List[str] = Field(default_factory=lambda: ["standard", "clone"])
    option_panel: Dict[str, ModelOptionSpec] = Field(default_factory=lambda: {
        "standard": ModelOptionSpec(
            notice_banner=NoticeBannerSpec(
                level="info",
                message="Using high-fidelity Standard Model."
            ),
            voice_type="select",
            speed_type="slider"
        )
    })
```

---

## 4. Custom Voice Metadata Schema (`voice_metadata_schema`)

When users upload audio to create a cloned voice, the modal form can adapt to demand custom attributes required by your model (e.g., Accent, Age, Pitch tier, Style):

```python
VoiceMetadataFieldSpec(key="name", label="Voice Name", type="text", required=True),
VoiceMetadataFieldSpec(key="gender", label="Gender", type="select", options=["Male", "Female", "Other"]),
VoiceMetadataFieldSpec(key="accent", label="Accent / Region", type="select", options=["North American", "British", "Australian", "Other"]),
VoiceMetadataFieldSpec(key="pitch_tier", label="Pitch Tier", type="select", options=["Low", "Medium", "High"])
```

---

## 5. Text Normalization & Cleaning Rules (`auto_format`)

To ensure clean text before feeding into your AI model, define your regex normalization rules in `DEFAULT_AUTO_FORMAT_RULES`:

```python
DEFAULT_AUTO_FORMAT_RULES = [
    AutoFormatRule(find=r"\r\n", replace="\n"),
    AutoFormatRule(find=r"\n{3,}", replace="\n\n"),
    AutoFormatRule(find=r"\u00D0", replace="\u0110"),  # Fix unicode Eth to Đ
    AutoFormatRule(find=r"([a-zA-ZÀ-ỹ])\-([a-zA-ZÀ-ỹ])", replace=r"\1 \2"), # Unhyphenate
    AutoFormatRule(find=r"[⁰¹²³⁴⁵⁶⁷⁸⁹₀₁₂₃₄₅₆₇₈₉]", replace=""), # Strip superscript footnotes
    AutoFormatRule(find=r"[^a-zA-Z0-9 \n\t\r.,?!;:\-\"'()\[\]%/“”‘’À-ỹ]", replace="") # Strip strange characters
]
```

---

🎉 **Congratulations!** Your AI model is now fully integrated with dynamic UI controls and automatic text hygiene!
