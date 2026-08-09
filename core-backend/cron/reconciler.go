package cron

import (
	"context"
	"log"
	"time"

	"core-backend/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// reconcileInterval là khoảng giữa hai lượt quét chunk mồ côi.
//
// Ngắn hơn sweep storage (1 tiếng) vì một chunk bị kẹt ảnh hưởng trực tiếp đến trải nghiệm
// người dùng (job không bao giờ kết thúc), trong khi một tệp tạm sót chỉ phình kho.
const reconcileInterval = 5 * time.Minute

// reconcileStaleChunks gọi ReconcileStaleChunks trên DB để chuyển chunk pending/processing
// quá hạn thành error. Bỏ qua nếu DB chưa được khởi tạo (queries nil) hoặc lỗi truy vấn.
//
// Đây là lớp cứu hộ cuối: không có gì bên ngoài cron sẽ tự động chuyển trạng thái cho một
// chunk mồ côi đã mất hàng đợi. Worker và web chỉ ghi terminal status khi chúng chạm được
// tới chunk — nếu job nằm trong stream mất tích (Redis restart) hoặc worker chết giữa chừng,
// DB không bao giờ thấy một lượt ghi nào khác từ enqueue đến lúc cron quét.
func reconcileStaleChunks(staleAfter time.Duration) {
	if db.Queries == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	staleBefore := pgtype.Timestamptz{Time: time.Now().Add(-staleAfter), Valid: true}
	n, err := db.Queries.ReconcileStaleChunks(ctx, staleBefore)
	if err != nil {
		log.Printf("Reconcile chunk mồ côi thất bại: %v", err)
		return
	}
	if n > 0 {
		log.Printf("Reconcile: đã chuyển %d chunk mồ côi (pending/processing) → error", n)
	}
}

// ReconcileLoop chạy vòng lặp reconcile cho tới khi ctx bị huỷ.
//
// Dùng trong cron.Run (chạy goroutine riêng). Nếu không có kết nối DB thì bỏ qua im lặng.
func ReconcileLoop(ctx context.Context, staleAfter time.Duration) {
	if db.Queries == nil {
		return
	}

	// Chạy một lượt ngay khi khởi động để dọn dẹp chunk tồn đọng từ lần trước.
	reconcileStaleChunks(staleAfter)

	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcileStaleChunks(staleAfter)
		}
	}
}