package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"core-backend/storage"
)

// TestVoiceKey_ContainsTraversal khoá lại lớp phòng vệ ở tầng dựng khoá.
//
// Handler đã kiểm model_id theo Manifest, nhưng hàm dựng khoá không được tin điều đó: một lần
// quên kiểm ở tầng trên là đủ để ghi ra ngoài nhánh của người dùng, với nội dung do người gửi
// kiểm soát.
func TestVoiceKey_ContainsTraversal(t *testing.T) {
	cases := []struct {
		name     string
		modeID   string
		userID   string
		filename string
	}{
		{"mode trèo lên", "../../../../etc", "user-1", "a.wav"},
		{"mode là dấu hai chấm", "..", "user-1", "a.wav"},
		{"user trèo lên", "clone", "../../root", "a.wav"},
		{"dấu gạch chéo giữa mode", "clone/../../etc", "user-1", "a.wav"},
		{"tên tệp trèo lên", "clone", "user-1", "../../../passwd"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := storage.VoiceKey(c.modeID, c.userID, c.filename)

			for _, seg := range strings.Split(got, "/") {
				if seg == ".." {
					t.Errorf("VoiceKey(%q,%q,%q) = %q — còn thành phần trèo lên",
						c.modeID, c.userID, c.filename, got)
				}
			}
			if strings.HasPrefix(got, "/") {
				t.Errorf("VoiceKey(...) = %q — khoá không được là đường dẫn tuyệt đối", got)
			}
		})
	}
}

// TestVoiceKey_KeepsValidNames xác nhận lớp làm sạch không phá tên hợp lệ.
func TestVoiceKey_KeepsValidNames(t *testing.T) {
	got := storage.VoiceKey("zero_shot_clone", "019f90a6-d5c8-7795-bae5-de6ae408d880", "abc.wav")
	want := "zero_shot_clone/019f90a6-d5c8-7795-bae5-de6ae408d880/voice/abc.wav"
	if got != want {
		t.Errorf("khoá hợp lệ bị đổi: got %q, want %q", got, want)
	}
}

// TestLocalStore_RejectsEscapingKeys là lưới an toàn cuối cùng.
//
// Kiểm ở tầng kho chứ không tầng khoá: kể cả khi một người gọi tự dựng khoá thay vì dùng
// VoiceKey/TempKey, kho vẫn không được phép ghi ra ngoài gốc của nó.
func TestLocalStore_RejectsEscapingKeys(t *testing.T) {
	root := t.TempDir()
	s, err := storage.NewLocalStore(root)
	if err != nil {
		t.Fatalf("không dựng được kho: %v", err)
	}
	ctx := context.Background()

	outside := filepath.Join(filepath.Dir(root), "escaped.txt")

	for _, key := range []string{
		"../escaped.txt",
		"../../escaped.txt",
		"temp/../../escaped.txt",
		"/etc/escaped.txt",
	} {
		// Ghi được hay bị từ chối đều chấp nhận; điều KHÔNG được phép là tệp xuất hiện ngoài gốc.
		_ = s.Put(ctx, key, []byte("x"))

		if _, err := os.Stat(outside); err == nil {
			os.Remove(outside)
			t.Fatalf("khoá %q đã ghi được ra ngoài gốc kho", key)
		}
	}
}

// TestLocalStore_RoundTrip kiểm những gì handler thực sự dựa vào.
func TestLocalStore_RoundTrip(t *testing.T) {
	s, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("không dựng được kho: %v", err)
	}
	ctx := context.Background()
	const key = "temp/abc.wav"

	if ok, _ := s.Exists(ctx, key); ok {
		t.Error("khoá chưa ghi mà đã tồn tại")
	}
	if _, err := s.Get(ctx, key); err != storage.ErrNotFound {
		t.Errorf("đọc khoá chưa có phải trả ErrNotFound, nhận %v", err)
	}

	if err := s.Put(ctx, key, []byte("hello")); err != nil {
		t.Fatalf("ghi: %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil || string(got) != "hello" {
		t.Errorf("đọc lại: %q, %v", got, err)
	}

	// Xoá khoá không tồn tại không phải lỗi: người gọi muốn nó biến mất, và nó đã biến mất.
	if err := s.Delete(ctx, "temp/khong-ton-tai.wav"); err != nil {
		t.Errorf("xoá khoá không tồn tại không nên là lỗi: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Errorf("xoá: %v", err)
	}
	if ok, _ := s.Exists(ctx, key); ok {
		t.Error("khoá vẫn còn sau khi xoá")
	}
}
