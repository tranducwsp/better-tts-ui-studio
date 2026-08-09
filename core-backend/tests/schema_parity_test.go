package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Hai đặc tả Manifest phía Python: engine thật và example dùng để chạy frontend không cần GPU.
const (
	realSchemaPath = "../../core-tts/schemas.py"
	mockSchemaPath = "../../core-tts-example/schemas.py"
)

// TestPythonSchemaParity giữ example engine đồng bộ với engine thật.
//
// Example engine là bản tham khảo contract cho AI engineer — nó chỉ nên khai những trường
// nền tảng thực sự gửi/nhận. Engine thật có thể có thêm trường nội bộ (output_format,
// ref_voice_id, task_id, voice) mà nền tảng không dùng — example không cần copy.
//
// Ngược lại, example có thể khai trường nền tảng gửi mà engine thật chưa có (ví dụ emotion)
// khi engine thật chưa cập nhật — đó là tín hiệu engine thật cần bắt kịp.
//
// Kiểm bằng cách đọc văn bản hai tệp thay vì chạy Python: không có pytest trong repo, và một
// bài test chỉ chạy khi ai đó nhớ chạy nó thì không chặn được sai lệch. Đặt ở đây để nó nằm
// trong `go test ./...` cùng mọi thứ khác.
func TestPythonSchemaParity(t *testing.T) {
	real := parsePydanticClasses(t, realSchemaPath)
	mock := parsePydanticClasses(t, mockSchemaPath)

	if len(real) == 0 || len(mock) == 0 {
		t.Fatalf("không đọc được class nào (real=%d, mock=%d) — có thể đường dẫn đã đổi",
			len(real), len(mock))
	}

	// Trường chỉ có ở engine thật — nền tảng không gửi, example không cần khai.
	realOnlyFields := map[string]map[string]string{
		"SynthesizeRequest": {
			"output_format": "engine thật dùng nội bộ, nền tảng không gửi",
			"ref_voice_id":  "engine thật dùng nội bộ, nền tảng không gửi",
			"task_id":       "engine thật dùng nội bộ, nền tảng không gửi",
			"voice":         "engine thật dùng nội bộ, nền tảng không gửi",
		},
	}

	// Trường chỉ có ở example — nền tảng gửi nhưng engine thật chưa khai.
	mockOnlyFields := map[string]map[string]string{
		"SynthesizeRequest": {
			"emotion": "nền tảng gửi khi mode khai supports_emotion",
		},
	}

	// Class chỉ có ở engine thật — example không cần vì nền tảng không dùng.
	realOnlyClasses := map[string]string{
		"TaskStatusResponse": "nền tảng có vòng đời task riêng, không gọi /tasks/{id}",
	}

	for _, name := range sortedKeys(real) {
		mockFields, ok := mock[name]
		if !ok {
			if _, isExpected := realOnlyClasses[name]; isExpected {
				continue // known: example không cần class này
			}
			t.Errorf("example thiếu class %s mà engine thật khai", name)
			continue
		}
		for _, f := range sortedKeys(real[name]) {
			if _, ok := mockFields[f]; !ok {
				if _, isExpected := realOnlyFields[name][f]; isExpected {
					continue // known: example không cần trường này
				}
				t.Errorf("%s: example thiếu trường %q mà engine thật khai", name, f)
			}
		}
	}

	// Chiều ngược lại: example khai thêm trường mà engine thật chưa có.
	// Đây là tín hiệu engine thật cần cập nhật — không phải lỗi của example.
	for _, name := range sortedKeys(mock) {
		realFields, ok := real[name]
		if !ok {
			t.Errorf("example khai class %s mà engine thật không có", name)
			continue
		}
		for _, f := range sortedKeys(mock[name]) {
			if _, ok := realFields[f]; !ok {
				if _, isExpected := mockOnlyFields[name][f]; isExpected {
					continue // known: nền tảng gửi nhưng engine thật chưa khai
				}
				t.Errorf("%s: example khai thêm trường %q không có ở engine thật", name, f)
			}
		}
	}
}

// TestMockDeclaresVoiceModes kiểm đúng trường đã gây ra sai lệch lần trước.
//
// Tách khỏi bài so sánh chung ở trên vì lý do khác nhau: ở đây không phải "hai tệp phải
// giống nhau" mà là "trường này bắt buộc, không được để có giá trị mặc định". Nếu ai đó thêm
// lại `= Field(default_factory=list)` thì Pydantic nhận một giọng không khai mode, và cách
// nền tảng lọc sẽ âm thầm trả về danh sách rỗng.
func TestMockDeclaresVoiceModes(t *testing.T) {
	for _, path := range []string{realSchemaPath, mockSchemaPath} {
		src := readSchema(t, path)

		block, ok := classBlock(src, "VoiceInfo")
		if !ok {
			t.Fatalf("%s: không tìm thấy class VoiceInfo", path)
		}

		line := ""
		for _, l := range strings.Split(block, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "modes:") {
				line = strings.TrimSpace(l)
				break
			}
		}
		if line == "" {
			t.Errorf("%s: VoiceInfo phải khai `modes`", filepath.Base(path))
			continue
		}
		if strings.Contains(line, "=") {
			t.Errorf("%s: `modes` phải là trường bắt buộc, không có mặc định — đang là %q",
				filepath.Base(path), line)
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

// classBlock trả về phần thân của một class, tới trước class kế tiếp.
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

// parsePydanticClasses gom tên trường có chú thích kiểu của từng class trong tệp.
//
// Chỉ nhận dòng thụt đúng bốn khoảng trắng, nên biến cục bộ trong method không bị tính là
// trường. Đủ cho hai tệp đặc tả thuần khai báo này; nếu chúng về sau có logic thật thì bài
// test cần một bộ phân tích thật thay vì regex.
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

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
