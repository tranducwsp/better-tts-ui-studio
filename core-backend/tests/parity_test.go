package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"core-backend/types"
)

// Bộ ca dùng chung với frontend. Nếu thêm ca mới, sửa docs/capability-resolution-cases.json
// và cả hai phía tự nhận — đó là điểm của việc để dữ liệu ngoài mã nguồn.
const fixturePath = "../../docs/capability-resolution-cases.json"

type parityFixture struct {
	Manifest types.UniversalManifest `json:"manifest"`
	Cases    []struct {
		Why    string          `json:"why"`
		Mode   string          `json:"mode"`
		Expect map[string]bool `json:"expect"`
	} `json:"cases"`
	PlatformDefaults     map[string]bool `json:"platform_defaults"`
	DefaultMaxTextLength int             `json:"default_max_text_length"`
}

func loadFixture(t *testing.T) parityFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(fixturePath))
	if err != nil {
		t.Fatalf("không đọc được bộ ca dùng chung: %v", err)
	}
	var f parityFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("bộ ca dùng chung không hợp lệ: %v", err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("bộ ca rỗng")
	}
	return f
}

func asMap(r types.ResolvedCapabilities) map[string]bool {
	return map[string]bool{
		"supports_preset_voices": r.SupportsPresetVoices,
		"supports_cloning":       r.SupportsCloning,
		"supports_voice_saving":  r.SupportsVoiceSaving,
		"supports_streaming":     r.SupportsStreaming,
		"supports_speed":         r.SupportsSpeed,
		"supports_pitch":         r.SupportsPitch,
		"supports_emotion":       r.SupportsEmotion,
		"supports_ssml":          r.SupportsSsml,
	}
}

// TestResolverMatchesSharedCases kiểm tra resolver Go trên đúng bộ ca mà frontend dùng.
// Hai resolver hiện thực cùng một quy tắc ở hai ngôn ngữ; không có ràng buộc nào của trình
// biên dịch giữ chúng khớp nhau, nên bài test này là thứ duy nhất bắt được lệch.
func TestResolverMatchesSharedCases(t *testing.T) {
	f := loadFixture(t)

	for _, c := range f.Cases {
		got := asMap(f.Manifest.ResolveCapabilities(c.Mode))
		for key, want := range c.Expect {
			if got[key] != want {
				t.Errorf("mode %q (%s): %s = %v, mong đợi %v", c.Mode, c.Why, key, got[key], want)
			}
		}
	}
}

// TestPlatformDefaultsMatchFixture bắt trường hợp ai đó đổi hằng số mặc định ở một phía mà
// quên phía kia — bộ ca là bản ghi chung của giá trị đã thống nhất.
func TestPlatformDefaultsMatchFixture(t *testing.T) {
	f := loadFixture(t)

	got := asMap(types.PlatformDefaultCapabilities)
	for key, want := range f.PlatformDefaults {
		if got[key] != want {
			t.Errorf("mặc định nền tảng %s = %v, bộ ca ghi %v", key, got[key], want)
		}
	}

	var nilManifest *types.UniversalManifest
	if nilManifest.ChunkSize() != f.DefaultMaxTextLength {
		t.Errorf("ChunkSize() khi chưa có manifest = %d, bộ ca ghi %d",
			nilManifest.ChunkSize(), f.DefaultMaxTextLength)
	}
}
