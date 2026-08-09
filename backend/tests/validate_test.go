package tests

import (
	"strings"
	"testing"

	"backend/state"
	"backend/types"
)

func boolPtr(b bool) *bool { return &b }

// Manifest không có mode nào thì mọi lời gọi resolver sau đó chỉ là phỏng đoán, nên đây là
// lỗi chí tử chứ không phải cảnh báo.
func TestValidateRejectsManifestWithoutModes(t *testing.T) {
	m := &types.UniversalManifest{EngineID: "e", EngineName: "E"}

	errs, _ := m.Validate()
	if len(errs) == 0 {
		t.Fatal("manifest không có mode nào lẽ ra phải bị từ chối")
	}
}

func TestValidateRejectsDuplicateAndEmptyModeIDs(t *testing.T) {
	cases := []struct {
		name  string
		modes []types.EngineModeSpec
	}{
		{
			// Resolver dừng ở mode khớp đầu tiên, nên bản thứ hai vừa vô hình vừa cho thấy
			// người khai tưởng mình đang cấu hình một thứ khác.
			name: "trùng id",
			modes: []types.EngineModeSpec{
				{ID: "standard", Name: "A"},
				{ID: "standard", Name: "B"},
			},
		},
		{
			name:  "id rỗng",
			modes: []types.EngineModeSpec{{ID: "", Name: "Không tên"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &types.UniversalManifest{SupportedModes: tc.modes}
			if errs, _ := m.Validate(); len(errs) == 0 {
				t.Error("lẽ ra phải bị từ chối")
			}
		})
	}
}

// Manifest lành mạnh phải đi qua sạch, không lỗi và không cảnh báo — nếu không, cảnh báo sẽ
// thành tiếng ồn mà người vận hành học cách bỏ qua.
func TestValidateAcceptsCoherentManifest(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{ID: "standard", Name: "Standard"},
			{
				ID:   "clone",
				Name: "Clone",
				Capabilities: types.EngineCapabilities{
					SupportsCloning:      boolPtr(true),
					SupportsPresetVoices: boolPtr(false),
				},
			},
		},
		AudioSpec: types.AudioSpec{
			SupportedFormats:      []string{"wav", "mp3"},
			SupportedSampleRates:  []int{24000},
			DefaultFormat:         "wav",
			DefaultSampleRate:     24000,
			ReferenceAudioFormats: []string{"wav"},
			MaxUploadBytes:        100 << 20,
			MaxReferenceBytes:     10 << 20,
		},
	}

	errs, warnings := m.Validate()
	if len(errs) > 0 {
		t.Errorf("manifest lành mạnh mà có lỗi: %v", errs)
	}
	if len(warnings) > 0 {
		t.Errorf("manifest lành mạnh mà có cảnh báo: %v", warnings)
	}
}

// Các mâu thuẫn dưới đây resolve được, nên chỉ cảnh báo — nhưng phải cảnh báo, vì mỗi cái
// đều dẫn tới một hành vi sai mà người khai không thấy ở chỗ mình khai.
func TestValidateWarnsOnResolvableContradictions(t *testing.T) {
	cases := []struct {
		name     string
		manifest *types.UniversalManifest
		expect   string
	}{
		{
			name: "default_format ngoài supported_formats",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				AudioSpec: types.AudioSpec{
					SupportedFormats: []string{"wav"},
					DefaultFormat:    "opus",
				},
			},
			expect: "default_format",
		},
		{
			name: "default_sample_rate ngoài supported_sample_rates",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				AudioSpec: types.AudioSpec{
					SupportedSampleRates: []int{16000},
					DefaultSampleRate:    48000,
				},
			},
			expect: "default_sample_rate",
		},
		{
			// Trần sau khi cắt lớn hơn trần tệp gốc làm bước cắt trở nên vô nghĩa.
			name: "max_reference_bytes vượt max_upload_bytes",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				AudioSpec: types.AudioSpec{
					MaxUploadBytes:    5 << 20,
					MaxReferenceBytes: 50 << 20,
				},
			},
			expect: "max_reference_bytes",
		},
		{
			name: "model_sort trỏ mode không tồn tại",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				UISchema:       &types.UISchemaSpec{ModelSort: []string{"standard", "khong_ton_tai"}},
			},
			expect: "model_sort",
		},
		{
			name: "option_panel trỏ mode không tồn tại",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				UISchema: &types.UISchemaSpec{
					OptionPanel: map[string]types.ModelOptionSpec{"khong_ton_tai": {}},
				},
			},
			expect: "option_panel",
		},
		{
			// Danh sách khai ra nhưng sẽ không bao giờ hiện, nên người khai tưởng đã xong.
			name: "preset_voices ở mode đã tắt preset voices",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{
					ID:           "clone",
					Capabilities: types.EngineCapabilities{SupportsPresetVoices: boolPtr(false)},
				}},
				UISchema: &types.UISchemaSpec{
					OptionPanel: map[string]types.ModelOptionSpec{
						"clone": {PresetVoices: []types.PresetVoiceSpec{{ID: "v1", Name: "V1"}}},
					},
				},
			},
			expect: "preset_voices",
		},
		{
			name: "mode cloning không khai reference_audio_formats",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{
					ID:           "clone",
					Capabilities: types.EngineCapabilities{SupportsCloning: boolPtr(true)},
				}},
				// Engine cũng không khai, nên không có gì để kế thừa. Mặc định nền tảng chỉ áp
				// dụng khi resolve, còn ở đây ta muốn biết Engine có tự khai hay không.
				AudioSpec: types.AudioSpec{ReferenceAudioFormats: []string{}},
			},
			expect: "reference_audio_formats",
		},
		{
			name: "speed_range đảo ngược",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				Constraints: types.EngineConstraints{
					SpeedRange: types.RangeConstraint{Min: 2.0, Max: 0.5},
				},
			},
			expect: "speed_range",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs, warnings := tc.manifest.Validate()
			if len(errs) > 0 {
				t.Fatalf("lẽ ra chỉ cảnh báo, nhưng bị từ chối: %v", errs)
			}
			joined := strings.Join(warnings, " | ")
			if !strings.Contains(joined, tc.expect) {
				t.Errorf("không thấy cảnh báo chứa %q; nhận được: %s", tc.expect, joined)
			}
		})
	}
}

// Điểm quan trọng nhất của việc kiểm lúc nạp: một lần reload lỗi không được phép hạ một
// Engine đang chạy tốt xuống trạng thái tệ hơn lúc trước khi gọi.
func TestSetKeepsPreviousManifestWhenRejected(t *testing.T) {
	s := &state.EngineManifestState{}

	good := &types.UniversalManifest{
		EngineID:       "good",
		SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
	}
	if err := s.Set(good); err != nil {
		t.Fatalf("manifest hợp lệ mà bị từ chối: %v", err)
	}

	// Không có mode nào: chí tử.
	if err := s.Set(&types.UniversalManifest{EngineID: "broken"}); err == nil {
		t.Fatal("manifest không hợp lệ lẽ ra phải bị từ chối")
	}

	if got := s.Get(); got == nil || got.EngineID != "good" {
		t.Errorf("bản đang dùng phải được giữ nguyên sau một lần reload lỗi, nhận được %+v", got)
	}
}

func TestSetRejectsNilAndClearResets(t *testing.T) {
	s := &state.EngineManifestState{}

	if err := s.Set(nil); err == nil {
		t.Error("Set(nil) lẽ ra phải là lỗi; muốn dọn trạng thái thì dùng Clear")
	}

	if err := s.Set(&types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
	}); err != nil {
		t.Fatalf("manifest hợp lệ mà bị từ chối: %v", err)
	}
	if !s.IsLoaded() {
		t.Fatal("chuẩn bị dữ liệu thất bại")
	}

	s.Clear()
	if s.IsLoaded() {
		t.Error("Clear phải đưa trạng thái về chưa nạp")
	}
}
