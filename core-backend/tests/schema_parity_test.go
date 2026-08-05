package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Hai đặc tả Manifest phía Python: engine thật và mock dùng để chạy frontend không cần GPU.
const (
	realSchemaPath = "../../core-tts/schemas.py"
	mockSchemaPath = "../../core-tts-test/schemas.py"
)

// TestPythonSchemaParity giữ mock trùng hợp đồng với engine thật.
//
// Mock tồn tại để bắt lỗi trước khi chúng gặp engine thật, nên nó chỉ có giá trị khi khai
// đúng những gì engine khai. Nó đã từng trôi: `modes` được siết từ optional thành bắt buộc ở
// engine thật, mock không được sửa theo, nên nó trả mọi giọng cho mọi mode trong khi
// production lọc — tức mock không phát hiện được đúng loại lỗi nó sinh ra để phát hiện, và
// lỗi chỉ lộ khi chạy thật.
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

	for _, name := range sortedKeys(real) {
		mockFields, ok := mock[name]
		if !ok {
			t.Errorf("mock thiếu hẳn class %s", name)
			continue
		}
		for _, f := range sortedKeys(real[name]) {
			if _, ok := mockFields[f]; !ok {
				t.Errorf("%s: mock thiếu trường %q mà engine thật khai", name, f)
			}
		}
	}

	// Chiều ngược lại cũng đáng chặn: một trường chỉ có ở mock nghĩa là mock hứa điều mà
	// engine thật không cung cấp, nên thứ chạy được khi phát triển sẽ vỡ khi chạy thật.
	for _, name := range sortedKeys(mock) {
		realFields, ok := real[name]
		if !ok {
			t.Errorf("mock khai class %s mà engine thật không có", name)
			continue
		}
		for _, f := range sortedKeys(mock[name]) {
			if _, ok := realFields[f]; !ok {
				t.Errorf("%s: mock khai thêm trường %q không có ở engine thật", name, f)
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
