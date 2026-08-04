package types

import (
	"fmt"
	"slices"
)

// Validate kiểm tra Manifest có tự mâu thuẫn hay không, trước khi nó được nhận vào RAM.
//
// Nền tảng này đã chọn hỏng-ngay-lúc-khởi-động ở chỗ khác: một biến môi trường gõ sai làm
// tiến trình dừng lại kèm thông báo chỉ rõ biến nào, vì hỏng ngầm khó sửa hơn nhiều. Manifest
// thì trước đây được nhận vô điều kiện — Set() chỉ gán con trỏ — nên một Engine khai
// default_format="opus" mà supported_formats=["wav"] vẫn chạy, rồi phát ra tệp mang
// Content-Type không trình phát nào mở được, cách chỗ khai báo hàng chục phút gỡ lỗi.
//
// Trả về (lỗi chí tử, cảnh báo). Phân đôi vì hai loại sai khác hẳn nhau về hậu quả:
//
//   - Lỗi chí tử: resolver không thể cho ra câu trả lời đúng. Không có mode nào, hoặc mode
//     trùng ID — mọi thứ sau đó đều là phỏng đoán, nên từ chối nhận.
//   - Cảnh báo: resolve được nhưng Engine gần như chắc chắn khai sai. Vẫn nhận, và nói ra,
//     vì từ chối cả Manifest chỉ vì một mục ui_schema trỏ sai tên sẽ làm sập một Engine
//     lành mạnh vì lỗi chính tả ở phần trang trí.
func (m *UniversalManifest) Validate() (errs []error, warnings []string) {
	if m == nil {
		return []error{fmt.Errorf("manifest rỗng")}, nil
	}

	if len(m.SupportedModes) == 0 {
		errs = append(errs, fmt.Errorf("manifest không khai mode nào trong supported_modes"))
	}

	seen := make(map[string]struct{}, len(m.SupportedModes))
	for i, mode := range m.SupportedModes {
		if mode.ID == "" {
			errs = append(errs, fmt.Errorf("supported_modes[%d] không có id", i))
			continue
		}
		if _, dup := seen[mode.ID]; dup {
			// Resolver dừng ở mode khớp đầu tiên, nên bản thứ hai vừa vô hình vừa là dấu hiệu
			// người khai tưởng mình đang cấu hình một thứ khác.
			errs = append(errs, fmt.Errorf("mode %q khai trùng trong supported_modes", mode.ID))
			continue
		}
		seen[mode.ID] = struct{}{}
	}

	// Mọi kiểm tra dưới đây đọc giá trị ĐÃ hoà giải, không đọc thứ Engine viết ra: một mode
	// kế thừa default_format của Engine vẫn phải nhất quán với supported_formats mà chính nó
	// kế thừa, và chỉ resolver biết cặp ấy cuối cùng là gì.
	for _, mode := range m.SupportedModes {
		if mode.ID == "" {
			continue
		}
		spec := m.ResolveAudioSpec(mode.ID)
		caps := m.ResolveCapabilities(mode.ID)

		if len(spec.SupportedFormats) > 0 && !slices.Contains(spec.SupportedFormats, spec.DefaultFormat) {
			warnings = append(warnings, fmt.Sprintf(
				"mode %q: default_format %q không nằm trong supported_formats %v — tệp tải xuống sẽ mang Content-Type của một định dạng Engine không khai là sinh ra được",
				mode.ID, spec.DefaultFormat, spec.SupportedFormats))
		}

		if len(spec.SupportedSampleRates) > 0 && !slices.Contains(spec.SupportedSampleRates, spec.DefaultSampleRate) {
			warnings = append(warnings, fmt.Sprintf(
				"mode %q: default_sample_rate %d không nằm trong supported_sample_rates %v",
				mode.ID, spec.DefaultSampleRate, spec.SupportedSampleRates))
		}

		// Hai trần tồn tại cho hai thời điểm: tệp thô kéo vào, và clip sau khi cắt. Trần sau
		// cao hơn trần trước thì bước cắt trở nên vô nghĩa.
		if spec.MaxUploadBytes > 0 && spec.MaxReferenceBytes > spec.MaxUploadBytes {
			warnings = append(warnings, fmt.Sprintf(
				"mode %q: max_reference_bytes (%d) lớn hơn max_upload_bytes (%d) — clip sau khi cắt không thể lớn hơn tệp gốc",
				mode.ID, spec.MaxReferenceBytes, spec.MaxUploadBytes))
		}

		if caps.SupportsCloning && !declaresReferenceFormats(m, mode.ID) {
			warnings = append(warnings, fmt.Sprintf(
				"mode %q bật cloning nhưng không khai reference_audio_formats; nền tảng sẽ dùng mặc định %v, có thể không phải thứ Engine đọc được",
				mode.ID, PlatformDefaultAudioSpec.ReferenceAudioFormats))
		}
	}

	if m.Constraints.MaxTextLength < 0 {
		warnings = append(warnings, fmt.Sprintf(
			"constraints.max_text_length âm (%d); nền tảng sẽ dùng mặc định %d",
			m.Constraints.MaxTextLength, DefaultMaxTextLength))
	}

	if r := m.Constraints.SpeedRange; r.Min > 0 && r.Max > 0 && r.Min > r.Max {
		warnings = append(warnings, fmt.Sprintf(
			"constraints.speed_range có min (%.2f) lớn hơn max (%.2f)", r.Min, r.Max))
	}
	if r := m.Constraints.PitchRange; r.Min > r.Max {
		warnings = append(warnings, fmt.Sprintf(
			"constraints.pitch_range có min (%.2f) lớn hơn max (%.2f)", r.Min, r.Max))
	}

	warnings = append(warnings, m.validateUISchema(seen)...)
	return errs, warnings
}

// declaresReferenceFormats cho biết Engine có TỰ khai reference_audio_formats cho mode này
// hay không — ở cấp mode, hoặc kế thừa từ cấp Engine.
//
// Không hỏi ResolveAudioSpec: resolver luôn rơi về mặc định nền tảng nên nó không bao giờ
// trả về danh sách rỗng, và một kiểm tra dựa vào đó sẽ không bao giờ đúng.
func declaresReferenceFormats(m *UniversalManifest, modeID string) bool {
	for i := range m.SupportedModes {
		if m.SupportedModes[i].ID == modeID {
			if len(m.SupportedModes[i].AudioSpec.ReferenceAudioFormats) > 0 {
				return true
			}
			break
		}
	}
	return len(m.AudioSpec.ReferenceAudioFormats) > 0
}

// validateUISchema bắt những mục ui_schema trỏ tới mode không tồn tại.
//
// Chỉ cảnh báo, không phải lỗi: ui_schema là cách vẽ, nên một tên gõ sai ở đây làm mất một
// bảng điều khiển chứ không làm sai âm thanh phát ra. Nhưng im lặng thì người khai sẽ ngồi
// tìm mãi một bảng điều khiển không bao giờ hiện.
func (m *UniversalManifest) validateUISchema(modes map[string]struct{}) []string {
	if m.UISchema == nil {
		return nil
	}

	var out []string
	for _, id := range m.UISchema.ModelSort {
		if _, ok := modes[id]; !ok {
			out = append(out, fmt.Sprintf("ui_schema.model_sort nhắc mode %q không có trong supported_modes", id))
		}
	}

	for id, panel := range m.UISchema.OptionPanel {
		if _, ok := modes[id]; !ok {
			out = append(out, fmt.Sprintf("ui_schema.option_panel có mục cho mode %q không có trong supported_modes", id))
			continue
		}
		// Preset voices khai ở một mode mà chính nó tắt preset voices: nền tảng sẽ không hiện
		// danh sách, nên người khai tưởng mình đã cấu hình xong.
		if len(panel.PresetVoices) > 0 && !m.ResolveCapabilities(id).SupportsPresetVoices {
			out = append(out, fmt.Sprintf(
				"ui_schema.option_panel[%q] khai preset_voices nhưng mode này resolve supports_preset_voices=false", id))
		}
	}
	return out
}
