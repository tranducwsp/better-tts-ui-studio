package types

// EngineCapabilities describes what an Engine (or one of its Modes) can do.
//
// Every field uses a pointer to distinguish three states, not two:
//   - nil   — Mode says nothing, inherits the Engine-wide value.
//   - false — Mode explicitly declares NOT supported, even if the Engine says yes.
//   - true  — Mode explicitly declares supported.
//
// Do not read these fields directly. Use ResolveCapabilities to get the resolved value
// between the two layers; otherwise every consumer invents its own priority rule.
type EngineCapabilities struct {
	SupportsPresetVoices *bool `json:"supports_preset_voices,omitempty"`
	SupportsCloning      *bool `json:"supports_cloning,omitempty"`
	SupportsVoiceSaving  *bool `json:"supports_voice_saving,omitempty"`
	SupportsStreaming    *bool `json:"supports_streaming,omitempty"`
	SupportsSpeed        *bool `json:"supports_speed,omitempty"`
	SupportsPitch        *bool `json:"supports_pitch,omitempty"`
	SupportsEmotion      *bool `json:"supports_emotion,omitempty"`
	SupportsSsml         *bool `json:"supports_ssml,omitempty"`
}

// ResolvedCapabilities is the result after resolving the two layers — no more nil, ready to use.
type ResolvedCapabilities struct {
	SupportsPresetVoices bool `json:"supports_preset_voices"`
	SupportsCloning      bool `json:"supports_cloning"`
	SupportsVoiceSaving  bool `json:"supports_voice_saving"`
	SupportsStreaming    bool `json:"supports_streaming"`
	SupportsSpeed        bool `json:"supports_speed"`
	SupportsPitch        bool `json:"supports_pitch"`
	SupportsEmotion      bool `json:"supports_emotion"`
	SupportsSsml         bool `json:"supports_ssml"`
}

// EngineModeSpec describes a processing mode of the AI Engine (e.g. standard, fast, clone).
type EngineModeSpec struct {
	ID           string             `json:"id"`          // "standard", "fast", "clone"
	Name         string             `json:"name"`        // "Standard Neural", "Fast Streaming"
	Description  string             `json:"description"` // Short description of the mode
	Capabilities EngineCapabilities `json:"capabilities"`

	// Only contains fields that differ from the Engine-wide audio_spec.
	AudioSpec AudioSpec `json:"audio_spec"`
}

// RangeConstraint defines limits for a numeric parameter (Min, Max, Default, Step) for UI Sliders.
type RangeConstraint struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	Step    float64 `json:"step"`
}

// ChunkingSpec defines how to split text that exceeds MaxTextLength.
//
// Placed in Constraints rather than UISchema because it determines the actual data sent,
// not how it is displayed. Delimiters is a list of split points in descending priority order;
// the platform tries each on fragments that are still too long, then hard-cuts the remainder.
type ChunkingSpec struct {
	MaxChunkSize int      `json:"max_chunk_size,omitempty"`
	Delimiters   []string `json:"delimiters,omitempty"`
}

// EngineConstraints defines the technical and parameter constraints of the AI Engine.
type EngineConstraints struct {
	MaxTextLength     int             `json:"max_text_length"`
	SpeedRange        RangeConstraint `json:"speed_range"`
	PitchRange        RangeConstraint `json:"pitch_range"`
	SupportedEmotions []string        `json:"supported_emotions"`
	Chunking          ChunkingSpec    `json:"chunking"`
}

// AudioSpec defines the technical specifications of the output audio.
// AudioSpec describes the audio format the Engine produces and the reference audio constraints.
//
// Like EngineCapabilities, this block appears at two layers — Engine-wide and per-Mode.
// Empty fields (nil / empty string / 0) mean inherit from the layer above. Use ResolveAudioSpec
// to get the resolved value; reading directly will miss Mode overrides.
type AudioSpec struct {
	SupportedFormats     []string `json:"supported_formats,omitempty"`
	SupportedSampleRates []int    `json:"supported_sample_rates,omitempty"`
	DefaultFormat        string   `json:"default_format,omitempty"`
	DefaultSampleRate    int      `json:"default_sample_rate,omitempty"`

	// Reference audio INPUT. Unlike the fields above, which describe the format the Engine
	// OUTPUTS — an Engine that outputs MP3 does not necessarily read MP3.
	ReferenceAudioFormats []string `json:"reference_audio_formats,omitempty"`
	ReferenceAudioSeconds float64  `json:"reference_audio_seconds,omitempty"`

	// Two ceilings for two moments: MaxUploadBytes is the raw file the user drags in (higher,
	// because they may drag a long recording and only use a few seconds), MaxReferenceBytes is
	// the clip after trimming — what the Engine actually receives.
	MaxUploadBytes    int64 `json:"max_upload_bytes,omitempty"`
	MaxReferenceBytes int64 `json:"max_reference_bytes,omitempty"`
}

// AutoFormatRule defines an automatic Regex text replacement rule.
type AutoFormatRule struct {
	Find    string `json:"find"`
	Replace string `json:"replace"`
}

// NoticeBannerSpec defines a warning notice displayed on the UI.
type NoticeBannerSpec struct {
	Level   string `json:"level"`   // "info", "warning", "danger", "success"
	Message string `json:"message"` // Notice content
}

// InputPanelSpec configures the features for the text input panel.
//
// Every bool field uses a pointer to distinguish three states:
//   - nil   — Engine says nothing, inherits the platform default.
//   - false — Engine explicitly declares OFF, even if the default is on.
//   - true  — Engine explicitly declares ON.
//
// Do not read these fields directly. Use ResolveInputPanel to get the resolved value.
type InputPanelSpec struct {
	FileServe      *bool            `json:"file_serve,omitempty"`
	Closeable      *bool            `json:"closeable,omitempty"`
	FindMode       string           `json:"find_mode,omitempty"` // "expert", "express"
	ReplaceTool    *bool            `json:"replace_tool,omitempty"`
	EnableChunkBox *bool            `json:"enable_chunk_box,omitempty"`
	AutoFormat     []AutoFormatRule `json:"auto_format,omitempty"`
}

// ResolvedInputPanel is the result after resolution — no more nil, ready to use.
type ResolvedInputPanel struct {
	FileServe      bool             `json:"file_serve"`
	Closeable      bool             `json:"closeable"`
	FindMode       string           `json:"find_mode"`
	ReplaceTool    bool             `json:"replace_tool"`
	EnableChunkBox bool             `json:"enable_chunk_box"`
	AutoFormat     []AutoFormatRule `json:"auto_format,omitempty"`
}

// PlatformDefaultInputPanel applies when the Engine declares nothing. Must match
// PLATFORM_DEFAULT_INPUT_PANEL in frontend/src/lib/inputPanel.ts.
var PlatformDefaultInputPanel = ResolvedInputPanel{
	FileServe:      true,
	Closeable:      false,
	FindMode:       "expert",
	ReplaceTool:    true,
	EnableChunkBox: true,
}

// VoiceMetadataFieldSpec defines the configuration of a metadata field when creating a new voice sample.
type VoiceMetadataFieldSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // "text", "select"
	Required    bool     `json:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// PresetVoiceSpec is a voice bundled with the Engine, declared in the Manifest rather than via /voices.
type PresetVoiceSpec struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	SampleURL string            `json:"sample_url,omitempty"`
}

// ModelOptionSpec only describes HOW TO DRAW the control, not whether the control exists —
// that is determined by Capabilities. PitchType on a Mode with SupportsPitch = false has no effect.
type ModelOptionSpec struct {
	NoticeBanner        *NoticeBannerSpec        `json:"notice_banner,omitempty"`
	VoiceType           string                   `json:"voice_type,omitempty"`   // "select", "radio"
	SpeedType           string                   `json:"speed_type,omitempty"`   // "slider", "number"
	PitchType           string                   `json:"pitch_type,omitempty"`   // "slider", "number"
	EmotionType         string                   `json:"emotion_type,omitempty"` // "select", "radio"
	PresetVoices        []PresetVoiceSpec        `json:"preset_voices,omitempty"`
	VoiceMetadataSchema []VoiceMetadataFieldSpec `json:"voice_metadata_schema,omitempty"`
}

// UISchemaSpec contains the full dynamic UI layout configuration dictated by the Core Engine.
type UISchemaSpec struct {
	UIMode      string                     `json:"ui_mode,omitempty"` // "beauty", "fast"
	InputPanel  InputPanelSpec             `json:"input_panel"`
	ModelSort   []string                   `json:"model_sort,omitempty"`
	OptionPanel map[string]ModelOptionSpec `json:"option_panel,omitempty"`
}

// UniversalManifest is the complete standard blueprint representing any AI Engine.
type UniversalManifest struct {
	EngineID       string             `json:"engine_id"`
	EngineName     string             `json:"engine_name"`
	Version        string             `json:"version"`
	Provider       string             `json:"provider"`
	SupportedModes []EngineModeSpec   `json:"supported_modes"`
	Capabilities   EngineCapabilities `json:"capabilities"`
	Constraints    EngineConstraints  `json:"constraints"`
	AudioSpec      AudioSpec          `json:"audio_spec"`
	UISchema       *UISchemaSpec      `json:"ui_schema,omitempty"`
}

// The value applied when neither Mode nor Engine declares anything. Must match
// PLATFORM_DEFAULTS in frontend/src/lib/capabilities.ts — this pair is verified by tests on both sides.
var PlatformDefaultCapabilities = ResolvedCapabilities{
	SupportsPresetVoices: true,
	SupportsCloning:      false,
	SupportsVoiceSaving:  false,
	SupportsStreaming:    true,
	SupportsSpeed:        true,
	SupportsPitch:        false,
	SupportsEmotion:      false,
	SupportsSsml:         false,
}

// DefaultMaxTextLength is used when the Manifest has not been loaded. Must match DEFAULT_TEXT_LIMIT
// in frontend/src/lib/textLimits.ts.
const DefaultMaxTextLength = 3000

// pick returns the Mode-declared value if present, otherwise the Engine-level value, and finally the default.
func pick(mode, engine *bool, fallback bool) bool {
	if mode != nil {
		return *mode
	}
	if engine != nil {
		return *engine
	}
	return fallback
}

// ResolveCapabilities resolves a Mode's Capabilities against the Engine-wide Capabilities.
//
// This is the ONLY place allowed to decide "can this mode do X". Every handler, validator,
// and UI layer must ask through here, so the priority rule exists in a single copy.
//
// An empty modeID or one that matches no Mode returns the Engine-wide Capabilities.
func (m *UniversalManifest) ResolveCapabilities(modeID string) ResolvedCapabilities {
	if m == nil {
		return PlatformDefaultCapabilities
	}

	var mode EngineCapabilities
	for i := range m.SupportedModes {
		if m.SupportedModes[i].ID == modeID {
			mode = m.SupportedModes[i].Capabilities
			break
		}
	}

	e := m.Capabilities
	d := PlatformDefaultCapabilities
	return ResolvedCapabilities{
		SupportsPresetVoices: pick(mode.SupportsPresetVoices, e.SupportsPresetVoices, d.SupportsPresetVoices),
		SupportsCloning:      pick(mode.SupportsCloning, e.SupportsCloning, d.SupportsCloning),
		SupportsVoiceSaving:  pick(mode.SupportsVoiceSaving, e.SupportsVoiceSaving, d.SupportsVoiceSaving),
		SupportsStreaming:    pick(mode.SupportsStreaming, e.SupportsStreaming, d.SupportsStreaming),
		SupportsSpeed:        pick(mode.SupportsSpeed, e.SupportsSpeed, d.SupportsSpeed),
		SupportsPitch:        pick(mode.SupportsPitch, e.SupportsPitch, d.SupportsPitch),
		SupportsEmotion:      pick(mode.SupportsEmotion, e.SupportsEmotion, d.SupportsEmotion),
		SupportsSsml:         pick(mode.SupportsSsml, e.SupportsSsml, d.SupportsSsml),
	}
}

// ResolveInputPanel resolves the Engine's InputPanelSpec against the platform defaults.
//
// This is the ONLY place allowed to decide "does the input panel have feature X". Every handler
// and UI layer must ask through here, so the priority rule exists in a single copy.
func (m *UniversalManifest) ResolveInputPanel() ResolvedInputPanel {
	if m == nil || m.UISchema == nil {
		return PlatformDefaultInputPanel
	}
	ip := m.UISchema.InputPanel
	d := PlatformDefaultInputPanel
	return ResolvedInputPanel{
		FileServe:      pickBool(ip.FileServe, d.FileServe),
		Closeable:      pickBool(ip.Closeable, d.Closeable),
		FindMode:       pickStr(ip.FindMode, d.FindMode),
		ReplaceTool:    pickBool(ip.ReplaceTool, d.ReplaceTool),
		EnableChunkBox: pickBool(ip.EnableChunkBox, d.EnableChunkBox),
		AutoFormat:     ip.AutoFormat,
	}
}

// pickBool returns the Engine-declared value if present, otherwise the default.
func pickBool(v *bool, fallback bool) bool {
	if v != nil {
		return *v
	}
	return fallback
}

// pickStr returns the Engine-declared value if non-empty, otherwise the default.
func pickStr(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

// ChunkSize returns the size of a text chunk, clamped to MaxTextLength so it never produces
// a chunk the Engine itself would reject.
func (m *UniversalManifest) ChunkSize() int {
	if m == nil || m.Constraints.MaxTextLength <= 0 {
		return DefaultMaxTextLength
	}
	ceiling := m.Constraints.MaxTextLength
	if pref := m.Constraints.Chunking.MaxChunkSize; pref > 0 && pref < ceiling {
		return pref
	}
	return ceiling
}

// TextLimit returns the effective text length ceiling.
//
// An Engine declaring 0 (or negative) means "unspecified", not "unlimited": validate.go
// only warns rather than rejecting such a manifest, so if the checker reads
// Constraints.MaxTextLength directly and skips when it is zero, the ceiling silently
// disappears. Same default as ChunkSize, so the chunks the platform splits never exceed
// the limit it applies.
func (m *UniversalManifest) TextLimit() int {
	if m == nil || m.Constraints.MaxTextLength <= 0 {
		return DefaultMaxTextLength
	}
	return m.Constraints.MaxTextLength
}

// PlatformDefaultAudioSpec applies when neither Mode nor Engine declares anything.
//
// Must match the fallbacks in frontend/src/lib/audioSpec.ts.
var PlatformDefaultAudioSpec = AudioSpec{
	SupportedFormats:      []string{"wav"},
	SupportedSampleRates:  []int{24000},
	DefaultFormat:         "wav",
	DefaultSampleRate:     24000,
	ReferenceAudioFormats: []string{"wav"},
	ReferenceAudioSeconds: 5.0,
	MaxUploadBytes:        100 * 1024 * 1024,
	MaxReferenceBytes:     10 * 1024 * 1024,
}

// ResolveAudioSpec resolves a Mode's AudioSpec against the Engine-wide AudioSpec.
//
// Same rule as ResolveCapabilities: what the Mode declares takes priority, otherwise the
// Engine, and finally the platform default. Necessary because different Modes may run on
// different backends — one Mode using Edge TTS returns MP3 while another using a local model
// returns WAV, and assigning the wrong Content-Type produces a file no player can open.
func (m *UniversalManifest) ResolveAudioSpec(modeID string) AudioSpec {
	if m == nil {
		return PlatformDefaultAudioSpec
	}

	var mode AudioSpec
	for i := range m.SupportedModes {
		if m.SupportedModes[i].ID == modeID {
			mode = m.SupportedModes[i].AudioSpec
			break
		}
	}

	e := m.AudioSpec
	d := PlatformDefaultAudioSpec

	pickStrings := func(vals ...[]string) []string {
		for _, v := range vals {
			if len(v) > 0 {
				return v
			}
		}
		return nil
	}
	pickInts := func(vals ...[]int) []int {
		for _, v := range vals {
			if len(v) > 0 {
				return v
			}
		}
		return nil
	}
	pickStr := func(vals ...string) string {
		for _, v := range vals {
			if v != "" {
				return v
			}
		}
		return ""
	}
	pickFloat := func(vals ...float64) float64 {
		for _, v := range vals {
			if v > 0 {
				return v
			}
		}
		return 0
	}
	pickBytes := func(vals ...int64) int64 {
		for _, v := range vals {
			if v > 0 {
				return v
			}
		}
		return 0
	}
	pickInt := func(vals ...int) int {
		for _, v := range vals {
			if v > 0 {
				return v
			}
		}
		return 0
	}

	return AudioSpec{
		SupportedFormats:     pickStrings(mode.SupportedFormats, e.SupportedFormats, d.SupportedFormats),
		SupportedSampleRates: pickInts(mode.SupportedSampleRates, e.SupportedSampleRates, d.SupportedSampleRates),
		DefaultFormat:        pickStr(mode.DefaultFormat, e.DefaultFormat, d.DefaultFormat),
		DefaultSampleRate:    pickInt(mode.DefaultSampleRate, e.DefaultSampleRate, d.DefaultSampleRate),
		ReferenceAudioFormats: pickStrings(
			mode.ReferenceAudioFormats, e.ReferenceAudioFormats, d.ReferenceAudioFormats),
		ReferenceAudioSeconds: pickFloat(
			mode.ReferenceAudioSeconds, e.ReferenceAudioSeconds, d.ReferenceAudioSeconds),
		MaxUploadBytes:    pickBytes(mode.MaxUploadBytes, e.MaxUploadBytes, d.MaxUploadBytes),
		MaxReferenceBytes: pickBytes(mode.MaxReferenceBytes, e.MaxReferenceBytes, d.MaxReferenceBytes),
	}
}
