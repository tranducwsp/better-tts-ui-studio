package types

// EngineModeSpec mô tả chi tiết một chế độ xử lý của AI Engine (ví dụ: standard, fast, clone, zero_shot).
type EngineModeSpec struct {
	ID                   string `json:"id"`                    // "standard", "fast", "clone"
	Name                 string `json:"name"`                  // "Standard Neural", "Fast Streaming", "Zero-shot Clone"
	Description          string `json:"description"`           // Mô tả ngắn về mode
	SupportsPresetVoices bool   `json:"supports_preset_voices"` // Có hỗ trợ chọn giọng đọc có sẵn không
	SupportsCloning      bool   `json:"supports_cloning"`       // Có hỗ trợ upload file mẫu dùng tạm 1 lần không
	SupportsVoiceSaving  bool   `json:"supports_voice_saving"`  // Có hỗ trợ tạo & lưu giọng mới vào thư viện không
	SupportsStreaming    bool   `json:"supports_streaming"`     // Có hỗ trợ luồng phát âm thanh streaming không
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

// AutoFormatRule định nghĩa quy tắc thay thế văn bản Regex tự động.
type AutoFormatRule struct {
	Find    string `json:"find"`
	Replace string `json:"replace"`
}

// NoticeBannerSpec định nghĩa thông báo cảnh báo hiển thị trên UI.
type NoticeBannerSpec struct {
	Level   string `json:"level"`   // "info", "warning", "danger", "success"
	Message string `json:"message"` // Nội dung cảnh báo
}

// InputPanelSpec cấu hình các tính năng cho khung nhập văn bản.
type InputPanelSpec struct {
	FileServe       bool             `json:"file_serve"`
	Closeable       bool             `json:"closeable"`
	FindMode        string           `json:"find_mode"` // "expert", "express"
	ReplaceTool     bool             `json:"replace_tool"`
	EnableChunkBox  bool             `json:"enable_chunk_box"`
	MaxChunkSize    int              `json:"max_chunk_size,omitempty"`
	ChunkDelimiters []string         `json:"chunk_delimiters,omitempty"`
	AutoFormat      []AutoFormatRule `json:"auto_format,omitempty"`
}

// VoiceMetadataFieldSpec định nghĩa cấu hình một trường thông tin khi tạo giọng mẫu mới.
type VoiceMetadataFieldSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`                  // "text", "select"
	Required    bool     `json:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// ModelOptionSpec cấu hình các widget điều khiển dành riêng cho từng model.
type ModelOptionSpec struct {
	NoticeBanner        *NoticeBannerSpec        `json:"notice_banner,omitempty"`
	VoiceType           string                   `json:"voice_type,omitempty"` // "select", "radio"
	SpeedType           string                   `json:"speed_type,omitempty"` // "slider", "number", "stepped"
	PitchType           string                   `json:"pitch_type,omitempty"`
	EmotionType         string                   `json:"emotion_type,omitempty"`
	PresetVoices        []map[string]string      `json:"preset_voices,omitempty"`
	VoiceMetadataSchema []VoiceMetadataFieldSpec `json:"voice_metadata_schema,omitempty"`
}

// UISchemaSpec chứa toàn bộ cấu hình bố trí giao diện động do Core Engine quy định.
type UISchemaSpec struct {
	InputPanel  InputPanelSpec             `json:"input_panel"`
	ModelSort   []string                   `json:"model_sort,omitempty"`
	OptionPanel map[string]ModelOptionSpec `json:"option_panel,omitempty"`
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
	UISchema       *UISchemaSpec      `json:"ui_schema,omitempty"`
}
