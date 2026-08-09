package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const exampleSchemaPath = "../../core-tts-example/schemas.py"

// TestExampleDeclaresVoiceModes kiểm VoiceInfo.modes là trường bắt buộc.
//
// Nếu ai đó thêm lại `= Field(default_factory=list)` thì Pydantic nhận một giọng không khai
// mode, và cách nền tảng lọc sẽ âm thầm trả về danh sách rỗng.
func TestExampleDeclaresVoiceModes(t *testing.T) {
	src := readSchema(t, exampleSchemaPath)

	block, ok := classBlock(src, "VoiceInfo")
	if !ok {
		t.Fatalf("%s: không tìm thấy class VoiceInfo", exampleSchemaPath)
	}

	line := ""
	for _, l := range strings.Split(block, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "modes:") {
			line = strings.TrimSpace(l)
			break
		}
	}
	if line == "" {
		t.Errorf("%s: VoiceInfo phải khai `modes`", filepath.Base(exampleSchemaPath))
		return
	}
	if strings.Contains(line, "=") {
		t.Errorf("%s: `modes` phải là trường bắt buộc, không có mặc định — đang là %q",
			filepath.Base(exampleSchemaPath), line)
	}
}

// TestExampleSchemaParses kiểm example engine schemas.py có thể đọc được.
//
// Engine thật không còn nằm trong repo — AI engineer tự thay bằng engine của họ.
// Example engine là bản tham khảo contract nên phải hợp lệ.
func TestExampleSchemaParses(t *testing.T) {
	classes := parsePydanticClasses(t, exampleSchemaPath)
	if len(classes) == 0 {
		t.Fatalf("không đọc được class nào từ %s", exampleSchemaPath)
	}

	// Các class bắt buộc phải có trong example engine
	required := []string{"SynthesizeRequest", "VoiceInfo", "UniversalManifest"}
	for _, name := range required {
		if _, ok := classes[name]; !ok {
			t.Errorf("example thiếu class %s", name)
		}
	}

	// SynthesizeRequest phải có các trường nền tảng gửi
	srFields, ok := classes["SynthesizeRequest"]
	if !ok {
		return
	}
	for _, f := range []string{"text", "voice_id", "speed", "engine"} {
		if _, ok := srFields[f]; !ok {
			t.Errorf("SynthesizeRequest thiếu trường %q", f)
		}
	}
}

var (
	classRe = regexp.MustCompile(`(?m)^class\s+(\w+)\s*\(`)
	fieldRe = regexp.MustCompile(`(?m)^\s{4}(\w+)\s*:\s*\S`)
)

func readSchema(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("đọc %s: %v", path, err)
	}
	return string(raw)
}

func classBlock(src, name string) (string, bool) {
	locs := classRe.FindAllStringSubmatchIndex(src, -1)
	for i, loc := range locs {
		if src[loc[2]:loc[3]] != name {
			continue
		}
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return src[loc[0]:end], true
	}
	return "", false
}

func parsePydanticClasses(t *testing.T, path string) map[string]map[string]struct{} {
	t.Helper()
	src := readSchema(t, path)

	out := map[string]map[string]struct{}{}
	locs := classRe.FindAllStringSubmatchIndex(src, -1)
	for i, loc := range locs {
		name := src[loc[2]:loc[3]]
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}

		fields := map[string]struct{}{}
		for _, m := range fieldRe.FindAllStringSubmatch(src[loc[1]:end], -1) {
			fields[m[1]] = struct{}{}
		}
		out[name] = fields
	}
	return out
}
