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
func ModeDir(modeID, userID string) string {
	return filepath.Join(baseDir, modeID, userID, "voice")
}
