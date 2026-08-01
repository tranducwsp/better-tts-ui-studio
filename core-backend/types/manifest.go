package types

// EngineCapabilities mô tả những gì một Engine (hoặc một Mode của nó) làm được.
//
// Mọi trường dùng con trỏ để phân biệt ba trạng thái, chứ không phải hai:
//   - nil   — Mode không nói gì, kế thừa giá trị toàn Engine.
//   - false — Mode khẳng định KHÔNG hỗ trợ, kể cả khi Engine nói có.
//   - true  — Mode khẳng định có hỗ trợ.
//
// Không đọc trực tiếp các trường này. Dùng ResolveCapabilities để lấy giá trị đã hoà giải
// giữa hai tầng, nếu không mỗi nơi lại tự bịa một quy tắc ưu tiên khác nhau.
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

// ResolvedCapabilities là kết quả sau khi hoà giải hai tầng — không còn nil, dùng được ngay.
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

// EngineModeSpec mô tả một chế độ xử lý của AI Engine (ví dụ: standard, fast, clone).
type EngineModeSpec struct {
	ID           string             `json:"id"`          // "standard", "fast", "clone"
	Name         string             `json:"name"`        // "Standard Neural", "Fast Streaming"
	Description  string             `json:"description"` // Mô tả ngắn về mode
	Capabilities EngineCapabilities `json:"capabilities"`
}

// RangeConstraint định nghĩa giới hạn tham số số (Min, Max, Default, Step) cho UI Sliders.
type RangeConstraint struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	Step    float64 `json:"step"`
}

// ChunkingSpec quy định cách chia nhỏ văn bản vượt quá MaxTextLength.
//
// Đặt trong Constraints chứ không phải UISchema vì nó quyết định dữ liệu thực sự được gửi
// đi, không phải cách hiển thị. Delimiters là danh sách điểm cắt xếp theo mức ưu tiên
// giảm dần; nền tảng thử lần lượt trên các mảnh còn quá dài rồi cắt cứng phần còn lại.
type ChunkingSpec struct {
	MaxChunkSize int      `json:"max_chunk_size,omitempty"`
	Delimiters   []string `json:"delimiters,omitempty"`
}

// EngineConstraints định nghĩa các ràng buộc về kỹ thuật và tham số của AI Engine.
type EngineConstraints struct {
	MaxTextLength     int             `json:"max_text_length"`
	SpeedRange        RangeConstraint `json:"speed_range"`
	PitchRange        RangeConstraint `json:"pitch_range"`
	SupportedEmotions []string        `json:"supported_emotions"`
	Chunking          ChunkingSpec    `json:"chunking"`
}

// AudioSpec định nghĩa thông số kỹ thuật âm thanh xuất ra.
type AudioSpec struct {
	SupportedFormats     []string `json:"supported_formats"`
	SupportedSampleRates []int    `json:"supported_sample_rates"`
	DefaultFormat        string   `json:"default_format"`
	DefaultSampleRate    int      `json:"default_sample_rate"`
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
	FileServe      bool             `json:"file_serve"`
	Closeable      bool             `json:"closeable"`
	FindMode       string           `json:"find_mode"` // "expert", "express"
	ReplaceTool    bool             `json:"replace_tool"`
	EnableChunkBox bool             `json:"enable_chunk_box"`
	AutoFormat     []AutoFormatRule `json:"auto_format,omitempty"`
}

// VoiceMetadataFieldSpec định nghĩa cấu hình một trường thông tin khi tạo giọng mẫu mới.
type VoiceMetadataFieldSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // "text", "select"
	Required    bool     `json:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// PresetVoiceSpec là giọng đọc Engine kèm sẵn, khai ngay trong Manifest thay vì qua /voices.
type PresetVoiceSpec struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Gender       string   `json:"gender,omitempty"`
	Descriptions []string `json:"descriptions,omitempty"`
	SampleURL    string   `json:"sample_url,omitempty"`
}

// ModelOptionSpec chỉ mô tả CÁCH VẼ điều khiển, không quyết định điều khiển có tồn tại hay
// không — việc đó thuộc về Capabilities. PitchType trên một Mode có SupportsPitch = false
// sẽ không có tác dụng gì.
type ModelOptionSpec struct {
	NoticeBanner        *NoticeBannerSpec        `json:"notice_banner,omitempty"`
	VoiceType           string                   `json:"voice_type,omitempty"`   // "select", "radio"
	SpeedType           string                   `json:"speed_type,omitempty"`   // "slider", "number"
	PitchType           string                   `json:"pitch_type,omitempty"`   // "slider", "number"
	EmotionType         string                   `json:"emotion_type,omitempty"` // "select", "radio"
	PresetVoices        []PresetVoiceSpec        `json:"preset_voices,omitempty"`
	VoiceMetadataSchema []VoiceMetadataFieldSpec `json:"voice_metadata_schema,omitempty"`
}

// UISchemaSpec chứa toàn bộ cấu hình bố trí giao diện động do Core Engine quy định.
type UISchemaSpec struct {
	UIMode      string                     `json:"ui_mode,omitempty"` // "beauty", "fast"
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

// Giá trị áp dụng khi cả Mode lẫn Engine đều không khai. Phải khớp với PLATFORM_DEFAULTS
// trong frontend/src/lib/capabilities.ts — cặp này được đối chiếu bằng test ở cả hai phía.
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

// DefaultMaxTextLength dùng khi Manifest chưa nạp được. Phải khớp DEFAULT_TEXT_LIMIT
// trong frontend/src/lib/textLimits.ts.
const DefaultMaxTextLength = 3000

// pick trả về giá trị Mode khai nếu có, ngược lại lấy của Engine, cuối cùng là mặc định.
func pick(mode, engine *bool, fallback bool) bool {
	if mode != nil {
		return *mode
	}
	if engine != nil {
		return *engine
	}
	return fallback
}

// ResolveCapabilities hoà giải Capabilities của một Mode với Capabilities toàn Engine.
//
// Đây là nơi DUY NHẤT được phép quyết định "mode này có làm được X không". Mọi handler,
// validator và tầng UI đều phải hỏi qua đây, nhờ vậy quy tắc ưu tiên chỉ tồn tại một bản.
//
// modeID rỗng hoặc không khớp Mode nào thì trả về Capabilities toàn Engine.
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

// ChunkSize trả về kích thước một đoạn văn bản, đã kẹp theo MaxTextLength để không bao giờ
// sinh ra đoạn mà chính Engine sẽ từ chối.
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
