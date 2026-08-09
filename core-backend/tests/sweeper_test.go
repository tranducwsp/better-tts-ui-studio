package tests

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"core-backend/storage"
)

// newStore dựng một kho cục bộ trong thư mục tạm của bài kiểm thử.
func newStore(t *testing.T) (storage.Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := storage.NewLocalStore(root)
	if err != nil {
		t.Fatalf("không dựng được kho: %v", err)
	}
	return s, root
}

// writeAged ghi một đối tượng rồi lùi thời điểm sửa của nó.
//
// Đụng thẳng vào tệp thay vì qua Store vì Store không có API đặt thời gian — và bộ quét dọn
// quyết định theo chính thời gian đó, nên bài kiểm thử phải điều khiển được nó.
func writeAged(t *testing.T, s storage.Store, root, key string, size int, age time.Duration) {
	t.Helper()
	if err := s.Put(context.Background(), key, bytes.NewReader(make([]byte, size))); err != nil {
		t.Fatalf("ghi %s: %v", key, err)
	}
	full := filepath.Join(root, filepath.FromSlash(key))
	old := time.Now().Add(-age)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatalf("đổi thời gian %s: %v", key, err)
	}
}

func TestSweepRemovesOnlyExpired(t *testing.T) {
	s, root := newStore(t)
	ctx := context.Background()

	writeAged(t, s, root, "temp/old1.wav", 1000, 48*time.Hour)
	writeAged(t, s, root, "temp/old2.mp3", 2000, 25*time.Hour)
	writeAged(t, s, root, "temp/fresh.wav", 500, 1*time.Hour)
	writeAged(t, s, root, "temp/borderline.wav", 300, 23*time.Hour)

	// Giọng đã lưu nằm ngoài nhánh temp và không được phép bị chạm tới.
	writeAged(t, s, root, "clone/user-1/voice/keeper.wav", 900, 100*time.Hour)

	n, freed, err := storage.SweepTempObjects(ctx, s, 24*time.Hour)
	if err != nil {
		t.Fatalf("quét dọn: %v", err)
	}
	if n != 2 {
		t.Errorf("đã xoá %d đối tượng, mong đợi 2", n)
	}
	if freed != 3000 {
		t.Errorf("giải phóng %d byte, mong đợi 3000", freed)
	}

	for _, key := range []string{"temp/fresh.wav", "temp/borderline.wav"} {
		if ok, _ := s.Exists(ctx, key); !ok {
			t.Errorf("%s lẽ ra phải còn", key)
		}
	}
	for _, key := range []string{"temp/old1.wav", "temp/old2.mp3"} {
		if ok, _ := s.Exists(ctx, key); ok {
			t.Errorf("%s lẽ ra đã bị xoá", key)
		}
	}
	if ok, _ := s.Exists(ctx, "clone/user-1/voice/keeper.wav"); !ok {
		t.Error("giọng đã lưu lẽ ra không bị chạm tới")
	}
}

func TestSweepMissingPrefixIsNotAnError(t *testing.T) {
	s, _ := newStore(t)

	n, freed, err := storage.SweepTempObjects(context.Background(), s, time.Hour)
	if err != nil {
		t.Errorf("nhánh chưa tồn tại không nên là lỗi, nhận: %v", err)
	}
	if n != 0 || freed != 0 {
		t.Errorf("mong đợi 0/0, nhận %d/%d", n, freed)
	}
}

func TestSweepEmptyPrefix(t *testing.T) {
	s, root := newStore(t)
	writeAged(t, s, root, "temp/fresh.wav", 10, 0)

	n, _, err := storage.SweepTempObjects(context.Background(), s, time.Hour)
	if err != nil || n != 0 {
		t.Errorf("không có đối tượng nào quá hạn: n=%d err=%v", n, err)
	}
}
