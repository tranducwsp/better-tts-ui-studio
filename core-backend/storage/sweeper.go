package storage

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

// Chu kỳ quét mặc định. Thời gian giữ tập tin thì do TEMP_AUDIO_RETENTION_HOURS quyết định
// và được truyền vào từ main, nên không có hằng thứ hai ở đây — hai nguồn cho cùng một con
// số là cách chắc nhất để chúng lệch nhau.
const DefaultSweepInterval = 1 * time.Hour

// SweepTempFiles xoá các tập tin âm thanh tạm cũ hơn retention và trả về số tập tin đã xoá
// cùng số byte giải phóng.
//
// Chỉ quét đúng một cấp trong dir và bỏ qua thư mục con: storage/temp chứa tập tin phẳng
// theo task_id, còn giọng đã lưu nằm ở nhánh khác và không được phép xoá.
func SweepTempFiles(dir string, retention time.Duration) (int, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil // chưa có job nào chạy
		}
		return 0, 0, err
	}

	cutoff := time.Now().Add(-retention)
	var removed int
	var freed int64

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}

		size := info.Size()
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			// Tập tin có thể đang được đọc để trả về cho client; lần quét sau sẽ dọn.
			continue
		}
		removed++
		freed += size
	}

	return removed, freed, nil
}

// StartTempSweeper chạy vòng quét định kỳ ở goroutine nền.
//
// Quét ngay một lần lúc khởi động để dọn phần rác còn lại từ lần chạy trước — nếu tiến
// trình bị dừng đột ngột thì không ai xoá những tập tin đã sinh ra trong phiên đó.
func StartTempSweeper(dir string, retention, interval time.Duration) {
	sweep := func() {
		n, freed, err := SweepTempFiles(dir, retention)
		if err != nil {
			log.Printf("Temp sweep on %s failed: %v", dir, err)
			return
		}
		if n > 0 {
			log.Printf("Temp sweep: removed %d file(s), freed %.1f MB from %s", n, float64(freed)/(1024*1024), dir)
		}
	}

	go func() {
		sweep()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			sweep()
		}
	}()
}
