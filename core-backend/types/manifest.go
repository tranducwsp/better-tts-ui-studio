package types

// EngineModeSpec mô tả chi tiết một chế độ xử lý của AI Engine (ví dụ: standard, fast, clone, zero_shot).
type EngineModeSpec struct {
	ID          string `json:"id"`          // "standard", "fast", "clone"
	Name        string `json:"name"`        // "Standard Neural", "Fast Streaming", "Zero-shot Clone"
	Description string `json:"description"` // Mô tả ngắn về mode
}

// RangeConstraint định nghĩa giới hạn tham số số (Min, Max, Default, Step) cho UI Sliders.
type RangeConstraint struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	Step    float64 `json:"step"`
}

// EngineConstraints định nghĩa các ràng buộc về kỹ thuật và tham số của AI Engine.
type EngineConstraints struct {
	MaxTextLength     int             `json:"max_text_length"`
	SpeedRange        RangeConstraint `json:"speed_range"`
	PitchRange        RangeConstraint `json:"pitch_range"`
	SupportedEmotions []string        `json:"supported_emotions"`
}

// AudioSpec định nghĩa thông số kỹ thuật âm thanh xuất ra.
type AudioSpec struct {
	SupportedFormats     []string `json:"supported_formats"`
	SupportedSampleRates []int    `json:"supported_sample_rates"`
	DefaultFormat        string   `json:"default_format"`
	DefaultSampleRate    int      `json:"default_sample_rate"`
}

// EngineCapabilities định nghĩa các tính năng tính toán AI được hỗ trợ (dùng để render Dynamic UI).
type EngineCapabilities struct {
	SupportsPresetVoices bool `json:"supports_preset_voices"`
	SupportsCloning      bool `json:"supports_cloning"`
	SupportsStreaming    bool `json:"supports_streaming"`
	SupportsSpeed        bool `json:"supports_speed"`
	SupportsPitch        bool `json:"supports_pitch"`
	SupportsEmotion      bool `json:"supports_emotion"`
	SupportsSsml         bool `json:"supports_ssml"`
}

// UniversalManifest là bản thiết kế tiêu chuẩn đầy đủ đại diện cho bất kỳ AI Engine nào.
type UniversalManifest struct {
	EngineID       string             `json:"engine_id"` 
	EngineName     string             `json:"engine_name"`
	Version        string             `json:"version"`
	Provider       string             `json:"provider"`
	SupportedModes []EngineModeSpec   `json:"supported_modes"`
	Capabilities   EngineCapabilities `json:"capabilities"`
	Constraints    EngineConstraints  `json:"constraints"`
	AudioSpec      AudioSpec          `json:"audio_spec"`
}
