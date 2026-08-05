package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"core-backend/storage"
)

func write(t *testing.T, path string, size int, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	old := time.Now().Add(-age)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestSweepTempFilesRemovesOnlyExpired(t *testing.T) {
	dir := t.TempDir()

	write(t, filepath.Join(dir, "old1.wav"), 1000, 48*time.Hour)
	write(t, filepath.Join(dir, "old2.mp3"), 2000, 25*time.Hour)
	write(t, filepath.Join(dir, "fresh.wav"), 500, 1*time.Hour)
	write(t, filepath.Join(dir, "borderline.wav"), 300, 23*time.Hour)

	// Thư mục con phải được bỏ qua: giọng đã lưu không nằm trong diện xoá.
	sub := filepath.Join(dir, "saved")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(sub, "keeper.wav"), 900, 100*time.Hour)

	n, freed, err := storage.SweepTempFiles(dir, 24*time.Hour)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 2 {
		t.Errorf("đã xoá %d tập tin, mong đợi 2", n)
	}
	if freed != 3000 {
		t.Errorf("giải phóng %d byte, mong đợi 3000", freed)
	}

	for _, name := range []string{"fresh.wav", "borderline.wav"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s lẽ ra phải còn: %v", name, err)
		}
	}
	for _, name := range []string{"old1.wav", "old2.mp3"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s lẽ ra đã bị xoá", name)
		}
	}
	if _, err := os.Stat(filepath.Join(sub, "keeper.wav")); err != nil {
		t.Errorf("tập tin trong thư mục con lẽ ra không bị chạm: %v", err)
	}
}

func TestSweepTempFilesMissingDirIsNotAnError(t *testing.T) {
	n, freed, err := storage.SweepTempFiles(filepath.Join(t.TempDir(), "chua-ton-tai"), time.Hour)
	if err != nil {
		t.Errorf("thư mục chưa tồn tại không nên là lỗi, nhận: %v", err)
	}
	if n != 0 || freed != 0 {
		t.Errorf("mong đợi 0/0, nhận %d/%d", n, freed)
	}
}

func TestSweepTempFilesEmptyDir(t *testing.T) {
	n, _, err := storage.SweepTempFiles(t.TempDir(), time.Hour)
	if err != nil || n != 0 {
		t.Errorf("thư mục rỗng: n=%d err=%v", n, err)
	}
}
