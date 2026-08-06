package synth

import (
	"context"
	"log"
	"sync"
	"time"

	"core-backend/client"
	"core-backend/queue"
)

// inFlight đếm các lượt tổng hợp chạy ngay trong tiến trình này.
//
// Chỉ nhánh dự phòng không-Redis dùng tới: khi có hàng đợi, công việc thuộc về worker và tiến
// trình web không giữ gì để mà chờ.
var inFlight sync.WaitGroup

// GoLocal chạy một lượt tổng hợp trong tiến trình này và ghi nhận nó để lúc tắt còn chờ.
//
// Tồn tại vì `go synth.Run(...)` trần không ai theo dõi: http.Server.Shutdown chỉ chờ các
// kết nối đang mở, mà request tổng hợp đã trả task_id về từ lâu nên nó coi như đã rảnh. Tiến
// trình thoát, goroutine chết giữa chừng, và chunk nằm lại "processing" vĩnh viễn vì
// UpdateChunkStatus không bao giờ chạy tới. Worker đã có wg.Wait() cho đúng tình huống này;
// đường web thì chưa.
//
// context.Background chứ không phải context của request: công việc này sống lâu hơn request
// đã khởi động nó.
func GoLocal(tts *client.CoreTTSClient, job queue.Job) {
	inFlight.Add(1)
	go func() {
		defer inFlight.Done()
		Run(context.Background(), tts, job)
	}()
}

// WaitLocal chờ các lượt tổng hợp tại chỗ chạy nốt, tối đa timeout.
//
// Trả về false khi hết giờ mà vẫn còn việc: người gọi ghi log rồi thoát, vì chờ vô hạn biến
// một lần khởi động lại thành một tiến trình không bao giờ chết.
func WaitLocal(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		inFlight.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		log.Printf("Còn lượt tổng hợp chạy dở sau %s — thoát và để frontend gọi lại", timeout)
		return false
	}
}
