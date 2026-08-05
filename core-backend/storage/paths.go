package storage

import (
	"os"
	"path/filepath"
)

// baseDir là gốc lưu trữ đang có hiệu lực, đặt một lần lúc khởi động từ STORAGE_DIR.
//
// Handler dùng TempDir()/ModeDir() thay vì ghép "storage/..." tại chỗ. Trước đây mỗi nơi
// tự ghép chuỗi cứng, nên đặt STORAGE_DIR=/data khiến bộ quét dọn /data/temp trong khi
// tổng hợp vẫn ghi vào ./storage/temp — cấu hình có vẻ nhận nhưng không có tác dụng.
var baseDir = "storage"

// InitStorage chốt gốc lưu trữ và đảm bảo nó tồn tại.
func InitStorage(dir string) {
	if dir != "" {
		baseDir = dir
	}
	_ = os.MkdirAll(baseDir, 0755)
}

// TempDir là nơi chứa âm thanh tạm theo task, do bộ quét dọn định kỳ.
func TempDir() string { return filepath.Join(baseDir, "temp") }

// ModeDir là nơi chứa giọng người dùng đã lưu, tách theo mode rồi tới user.
//
// Cả hai thành phần đều đi qua safeSegment: người gọi được kỳ vọng đã kiểm modeID theo
// Manifest, nhưng một hàm dựng đường dẫn không nên tin điều đó. Một lần quên kiểm ở tầng
// handler là đủ để os.WriteFile ghi ra ngoài baseDir, nên chặn ở đây là chặn ở nơi hậu quả
// xảy ra.
func ModeDir(modeID, userID string) string {
	return filepath.Join(baseDir, safeSegment(modeID), safeSegment(userID), "voice")
}

// safeSegment thay mọi ký tự không nằm trong [A-Za-z0-9._-] bằng "_", và loại riêng ".."
// vì nó chỉ gồm các ký tự được phép nhưng vẫn trèo lên một cấp.
func safeSegment(s string) string {
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	out := []rune(s)
	for i, r := range out {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if !ok {
			out[i] = '_'
		}
	}
	return string(out)
}
