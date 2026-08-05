package storage

import (
	"context"
	"log"
	"time"
)

// Chu kỳ quét mặc định. Thời gian giữ tập tin thì do TEMP_AUDIO_RETENTION_HOURS quyết định
// và được truyền vào từ main, nên không có hằng thứ hai ở đây — hai nguồn cho cùng một con
// số là cách chắc nhất để chúng lệch nhau.
const DefaultSweepInterval = 1 * time.Hour

// sweepOpTimeout chặn một lượt quét, để bộ quét không treo mãi khi kho ở xa không trả lời.
const sweepOpTimeout = 2 * time.Minute

// SweepTempObjects xoá âm thanh tạm cũ hơn retention và trả về số đối tượng đã xoá cùng số
// byte giải phóng.
//
// Chỉ quét nhánh temp: giọng người dùng đã lưu nằm ở nhánh khác và không được phép xoá. Ràng
// buộc đó do List thi hành (không đệ quy), không phải do người gọi nhớ.
func SweepTempObjects(ctx context.Context, store Store, retention time.Duration) (int, int64, error) {
	objects, err := store.List(ctx, TempPrefix)
	if err != nil {
		return 0, 0, err
	}

	cutoff := time.Now().Add(-retention).Unix()
	var removed int
	var freed int64

	for _, o := range objects {
		if o.Modified > cutoff {
			continue
		}
		if err := store.Delete(ctx, o.Key); err != nil {
			// Tệp có thể đang được đọc để trả về cho client; lần quét sau sẽ dọn.
			continue
		}
		removed++
		freed += o.Size
	}

	return removed, freed, nil
}

// StartTempSweeper chạy vòng quét định kỳ ở goroutine nền.
//
// Quét ngay một lần lúc khởi động để dọn phần rác còn lại từ lần chạy trước — nếu tiến
// trình bị dừng đột ngột thì không ai xoá những tập tin đã sinh ra trong phiên đó.
func StartTempSweeper(store Store, retention, interval time.Duration) {
	sweep := func() {
		ctx, cancel := context.WithTimeout(context.Background(), sweepOpTimeout)
		defer cancel()

		n, freed, err := SweepTempObjects(ctx, store, retention)
		if err != nil {
			log.Printf("Quét dọn âm thanh tạm thất bại: %v", err)
			return
		}
		if n > 0 {
			log.Printf("Quét dọn: đã xoá %d tệp, giải phóng %.1f MB", n, float64(freed)/(1024*1024))
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
