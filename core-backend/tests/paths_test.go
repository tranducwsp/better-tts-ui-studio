package tests

import (
	"testing"

	"core-backend/storage"
)

// TestModeDir_ContainsTraversal khoá lại lớp phòng vệ ở tầng dựng đường dẫn.
//
// Handler đã kiểm model_id theo Manifest, nhưng ModeDir không được tin điều đó: một lần quên
// kiểm ở tầng trên là đủ để os.WriteFile ghi ra ngoài thư mục storage với nội dung do người
// gửi kiểm soát.
func TestModeDir_ContainsTraversal(t *testing.T) {
	cases := []struct {
		name   string
		modeID string
		userID string
	}{
		{"mode trèo lên", "../../../../etc", "user-1"},
		{"mode là dấu hai chấm", "..", "user-1"},
		{"user trèo lên", "clone", "../../root"},
		{"dấu gạch chéo giữa mode", "clone/../../etc", "user-1"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := storage.ModeDir(c.modeID, c.userID)

			// Không được có thành phần ".." nào sót lại, và đường dẫn phải còn nằm dưới gốc.
			for _, seg := range splitPath(got) {
				if seg == ".." {
					t.Errorf("ModeDir(%q, %q) = %q — vẫn còn thành phần trèo lên", c.modeID, c.userID, got)
				}
			}
			if !hasPrefixDir(got, "storage") {
				t.Errorf("ModeDir(%q, %q) = %q — đã ra ngoài gốc storage", c.modeID, c.userID, got)
			}
		})
	}
}

// TestModeDir_KeepsValidNames xác nhận lớp làm sạch không phá tên hợp lệ.
func TestModeDir_KeepsValidNames(t *testing.T) {
	got := storage.ModeDir("zero_shot_clone", "019f90a6-d5c8-7795-bae5-de6ae408d880")
	want := "storage/zero_shot_clone/019f90a6-d5c8-7795-bae5-de6ae408d880/voice"
	if got != want {
		t.Errorf("ModeDir hợp lệ bị đổi: got %q, want %q", got, want)
	}
}

func splitPath(p string) []string {
	var out []string
	cur := ""
	for _, r := range p {
		if r == '/' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}

func hasPrefixDir(p, prefix string) bool {
	return len(p) >= len(prefix) && p[:len(prefix)] == prefix
}
