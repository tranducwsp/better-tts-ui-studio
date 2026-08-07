package worker

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"core-backend/client"
	"core-backend/queue"
	"core-backend/state"
	"core-backend/synth"
)

// Run nhặt job từ hàng đợi và chạy cho tới khi nhận tín hiệu dừng.
// maxInFlight là trần số job chạy song song trong một worker; tổng tải là số worker nhân giá trị này.
//
// Không mở cổng nào: worker không phục vụ request, và mở một listener chỉ để healthcheck sẽ
// tạo ra một bề mặt không ai dùng. Trạng thái của nó nhìn được qua log và qua chính hàng đợi.
func Run(ttsClient *client.CoreTTSClient, maxInFlight int) {
	if maxInFlight < 1 {
		log.Fatal("WORKER_MAX_IN_FLIGHT phải lớn hơn 0")
	}
	if state.RedisClient == nil {
		log.Fatal("Worker cần Redis để nhận job, nhưng REDIS_URL chưa cấu hình hoặc không kết nối được.\n" +
			"Không có Redis thì chạy chế độ web là đủ: nó tự tổng hợp tại chỗ.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := queue.EnsureGroup(ctx); err != nil {
		log.Fatalf("Không tạo được nhóm tiêu thụ hàng đợi: %v", err)
	}

	name := consumerName()

	slots := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup

	log.Printf("Worker %q sẵn sàng, tối đa %d job song song", name, maxInFlight)

	err := queue.Consume(ctx, name, func(jobCtx context.Context, job queue.Job) {
		select {
		case slots <- struct{}{}:
		case <-jobCtx.Done():
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()

			// context.Background chứ không phải jobCtx: khi nhận tín hiệu dừng, job đang chạy
			// được chạy nốt thay vì bị cắt giữa chừng. Vòng lặp Consume đã dừng nhận job mới,
			// nên đây là phần đuôi hữu hạn.
			synth.Run(context.Background(), ttsClient, job)
		}()
	})
	if err != nil {
		log.Printf("Vòng đọc hàng đợi dừng: %v", err)
	}

	log.Println("Đang chờ các job dở dang chạy nốt...")
	wg.Wait()
	log.Println("Worker đã dừng gọn.")
}

// consumerName là tên định danh worker trong nhóm tiêu thụ.
//
// Hostname là tên container trong compose và tên pod trên k8s, nên nó vừa duy nhất giữa các
// replica vừa truy ngược được về tiến trình thật khi đọc log.
func consumerName() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "worker"
	}
	return name
}
