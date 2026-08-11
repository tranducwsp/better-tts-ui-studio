package testsupport

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// RepoRoot trả thư mục gốc repository dựa trên vị trí file helper này, không phụ thuộc working
// directory hay độ sâu của package test. Marker bắt lỗi nếu cây source bị đóng gói thiếu.
func RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("không xác định được vị trí testsupport")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "backend", "go.mod")); err != nil {
		t.Fatalf("không tìm thấy repo root từ %s: %v", file, err)
	}
	return root
}

// Path dựng đường dẫn tuyệt đối tới một tệp trong repository.
func Path(t *testing.T, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{RepoRoot(t)}, parts...)...)
}
