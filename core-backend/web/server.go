package web

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"core-backend/client"
	"core-backend/config"
	"core-backend/router"
	"core-backend/synth"
)

// localSynthDrainTimeout là thời gian chờ các lượt tổng hợp chạy tại chỗ khi tắt máy.
//
// Chỉ có nghĩa ở triển khai không Redis, nơi tiến trình web tự tổng hợp. Hữu hạn vì chờ vô
// hạn biến một lần khởi động lại thành một tiến trình không bao giờ chết.
const localSynthDrainTimeout = 30 * time.Second

// Run mở cổng HTTP và phục vụ cho tới khi nhận tín hiệu dừng.
func Run(cfg *config.Config, ttsClient *client.CoreTTSClient) {
	r := router.NewRouter(cfg, ttsClient)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: r,

		// ReadHeaderTimeout tách riêng khỏi ReadTimeout vì hai thứ này khác bản chất.
		//
		// ReadTimeout bao cả việc đọc body, mà trần upload là MAX_UPLOAD_SIZE_MB (mặc định
		// 256 MB): một người mạng chậm gửi tệp tham chiếu hợp lệ cần tới hàng phút, nên hạ nó
		// xuống là cắt ngang đúng những lượt tải hợp lệ nhất.
		//
		// Header thì ngược lại — nó phải tới trong vài giây với mọi client thật. Gộp hai thứ
		// vào một con số 120 giây nghĩa là mỗi kết nối nhỏ giọt một byte header giữ được một
		// goroutine suốt hai phút, và cách đó rẻ hơn nhiều so với dò mật khẩu mà rate limit
		// đang canh.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting Universal Control Plane Backend (Go Chi) server at http://%s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		// Không Fatalf: os.Exit bỏ qua mọi defer, và ngay dưới đây còn phải chờ các lượt tổng
		// hợp chạy tại chỗ. Hết giờ đóng listener không phải lý do để vứt công việc đang chạy.
		log.Printf("Server chưa đóng gọn trong hạn: %v", err)
	}

	// Chờ nhánh dự phòng không-Redis chạy nốt. Shutdown ở trên chỉ chờ kết nối HTTP, mà lượt
	// tổng hợp đã trả task_id về từ lâu nên nó không nhìn thấy — thoát luôn ở đây sẽ để chunk
	// mắc lại "processing" và người dùng thấy một job không bao giờ xong.
	synth.WaitLocal(localSynthDrainTimeout)

	log.Println("Server exited successfully")
}
