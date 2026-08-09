package cron

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"core-backend/storage"
)

// defaultInterval là khoảng giữa hai lượt quét. Thời gian GIỮ tập tin thì do
// TEMP_AUDIO_RETENTION_HOURS quyết định và truyền vào từ bootstrap.
const defaultInterval = 1 * time.Hour

// sweepOpTimeout chặn một lượt quét, để bộ quét không treo mãi khi kho ở xa không trả lời.
const sweepOpTimeout = 2 * time.Minute

// Run chạy vòng lặp dọn âm thanh tạm và Reconcile chunk mồ côi, giữ tiến trình sống tới khi
// nhận tín hiệu dừng.
//
// Vòng lặp là trách nhiệm của process cron, không phải của storage: lịch và tài nguyên là
// policy, còn storage chỉ cung cấp SweepTempObjects (thao tác dữ liệu thuần). Các tác vụ nền
// khác về sau cũng thêm vào đây.
//
// Không có healthcheck endpoint, không có cổng nào. Trạng thái nhìn qua log và qua việc
// kho temp/ không phình lên.
func Run(store storage.Store, retention, staleAfter time.Duration) {
	sweep := func() {
		ctx, cancel := context.WithTimeout(context.Background(), sweepOpTimeout)
		defer cancel()

		n, freed, err := storage.SweepTempObjects(ctx, store, retention)
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
		ticker := time.NewTicker(defaultInterval)
		defer ticker.Stop()
		for range ticker.C {
			sweep()
		}
	}()

	go ReconcileLoop(context.Background(), staleAfter)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
