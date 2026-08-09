package config

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const envExamplePath = "../../.env.example"

// TestEnvExampleIsUpToDate chạy lại generator vào tệp tạm và so với tệp đã commit.
//
// Không có bài test này, quên `go generate` sau khi sửa Settings sẽ để .env.example mô tả
// một cấu hình mà backend không còn dùng — đúng loại sai lệch âm thầm mà bảng đặc tả sinh
// ra để loại bỏ.
func TestEnvExampleIsUpToDate(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), ".env.example")

	cmd := exec.Command("go", "run", "../cmd/gen-env")
	cmd.Env = append(os.Environ(), "GEN_ENV_OUT="+tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("chạy gen-env thất bại: %v\n%s", err, out)
	}

	want, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("đọc tệp vừa sinh: %v", err)
	}
	got, err := os.ReadFile(envExamplePath)
	if err != nil {
		t.Fatalf("đọc .env.example đã commit: %v", err)
	}

	if string(got) != string(want) {
		t.Error(".env.example đã lệch khỏi config/settings.go.\n" +
			"Chạy: cd backend && go generate ./config")
	}
}

// TestEnvExampleCoversEverySetting đọc tệp đã commit như một người vận hành sẽ đọc, và
// kiểm tra không biến nào bị thiếu hay thừa.
func TestEnvExampleCoversEverySetting(t *testing.T) {
	f, err := os.Open(envExamplePath)
	if err != nil {
		t.Fatalf("mở .env.example: %v", err)
	}
	defer f.Close()

	inFile := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, _, ok := strings.Cut(line, "="); ok {
			inFile[strings.TrimSpace(k)] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("đọc .env.example: %v", err)
	}

	inSpec := map[string]bool{}
	for _, s := range Settings {
		inSpec[s.Key] = true
		if !inFile[s.Key] {
			t.Errorf("%s có trong Settings nhưng thiếu trong .env.example", s.Key)
		}
	}
	for k := range inFile {
		if !inSpec[k] {
			t.Errorf("%s có trong .env.example nhưng không có trong Settings", k)
		}
	}
}

// TestSecretsNeverCarryAValue bảo vệ điều dễ vô tình phá: một khoá thật bị điền vào tệp mẫu
// rồi commit.
func TestSecretsNeverCarryAValue(t *testing.T) {
	raw, err := os.ReadFile(envExamplePath)
	if err != nil {
		t.Fatalf("đọc .env.example: %v", err)
	}
	text := string(raw)

	for _, s := range Settings {
		if s.Kind != KindSecret {
			continue
		}
		if !strings.Contains(text, s.Key+"=\n") && !strings.HasSuffix(text, s.Key+"=") {
			t.Errorf("%s là bí mật nên phải để trống trong .env.example", s.Key)
		}
	}
}

// TestIntSettingsHaveUsableRanges bắt lỗi khai báo: mặc định nằm ngoài chính khoảng nó
// tuyên bố, hoặc min lớn hơn max. num() sẽ log.Fatalf lúc chạy, tức là chỉ lộ ra khi triển
// khai; bài test này làm nó lộ ra lúc build.
func TestIntSettingsHaveUsableRanges(t *testing.T) {
	for _, s := range Settings {
		if s.Kind != KindInt {
			continue
		}
		if s.Min > s.Max {
			t.Errorf("%s: Min (%d) lớn hơn Max (%d)", s.Key, s.Min, s.Max)
		}
		v, err := strconv.Atoi(s.Default)
		if err != nil {
			t.Errorf("%s: mặc định %q không phải số nguyên", s.Key, s.Default)
			continue
		}
		if v < s.Min || v > s.Max {
			t.Errorf("%s: mặc định %d nằm ngoài khoảng đã khai (%d đến %d)", s.Key, v, s.Min, s.Max)
		}
	}
}

// TestNoDuplicateKeys bắt trường hợp một khoá bị khai hai lần, khi đó lookup() lặng lẽ trả
// về bản đầu tiên và bản thứ hai không bao giờ có tác dụng.
func TestNoDuplicateKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Settings {
		if seen[s.Key] {
			t.Errorf("khoá %s được khai nhiều lần trong Settings", s.Key)
		}
		seen[s.Key] = true
	}
}

// TestReadByVarsAreNotLoaded giữ ranh giới: một biến đánh dấu ReadBy thuộc về dịch vụ
// khác, nên LoadConfig không được đọc nó. Nếu sau này backend cần dùng thật, hãy bỏ ReadBy
// thay vì để bảng nói một đằng còn mã nguồn làm một nẻo.
func TestReadByVarsAreNotLoaded(t *testing.T) {
	raw, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("đọc config.go: %v", err)
	}
	src := string(raw)

	for _, s := range Settings {
		if s.ReadBy == "" {
			continue
		}
		for _, call := range []string{`str("` + s.Key + `")`, `num("` + s.Key + `")`} {
			if strings.Contains(src, call) {
				t.Errorf("%s được đánh dấu ReadBy=%q nhưng LoadConfig vẫn gọi %s", s.Key, s.ReadBy, call)
			}
		}
	}
}

// TestRequiredSettingsAreEnforced xác nhận cờ Required thực sự chặn khởi động.
//
// Trước đây nó chỉ là trang trí: gen-env đọc để in dòng "BẮT BUỘC" vào .env.example, còn lúc
// chạy không ai kiểm — nên một triển khai thiếu SECRET_KEY vẫn lên bình thường bằng khoá
// ngẫu nhiên, và người vận hành chỉ biết nếu tình cờ đọc log.
//
// requireAll gọi log.Fatalf nên không gọi trực tiếp được trong test; thay vào đó kiểm chính
// tập hợp mà nó duyệt, và kiểm rằng nó được gọi từ LoadConfig.
func TestRequiredSettingsAreEnforced(t *testing.T) {
	raw, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("đọc config.go: %v", err)
	}
	src := string(raw)

	if !strings.Contains(src, "requireAll()") {
		t.Error("LoadConfig phải gọi requireAll(), nếu không cờ Required chỉ là chú thích")
	}

	// Ít nhất một biến phải được đánh dấu Required, nếu không bài test trên là vô nghĩa.
	found := false
	for _, s := range Settings {
		if s.Required {
			found = true
			break
		}
	}
	if !found {
		t.Error("không có biến nào Required — kiểm tra lại bảng Settings")
	}
}

// TestRequiredBackendVarsHaveNoDefault: một biến vừa Required vừa có Default là tự mâu thuẫn.
// requireAll chỉ xét ENV, nên Default sẽ không bao giờ dùng tới và chỉ gây hiểu sai khi đọc bảng.
func TestRequiredBackendVarsHaveNoDefault(t *testing.T) {
	for _, s := range Settings {
		if s.Required && s.Default != "" {
			t.Errorf("%s vừa Required vừa có Default %q — bỏ một trong hai", s.Key, s.Default)
		}
	}
}
