package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"core-backend/client"
	"core-backend/config"
	"core-backend/db"
	"core-backend/handlers"
	"core-backend/middleware"
	"core-backend/router"
	"core-backend/state"
	"core-backend/storage"
)

// manifestDiscoveryTimeout là thời gian chờ Engine trả về một Manifest hợp lệ lúc khởi động.
//
// Đủ rộng để Engine nạp xong mô hình khi cả hai cùng lên trong một lần `docker compose up`,
// nhưng vẫn hữu hạn: một tiến trình treo mãi ở bước khởi tạo không báo cho ai biết, trong khi
// một tiến trình thoát với thông báo rõ ràng thì mọi orchestrator đều thấy và khởi động lại.
const manifestDiscoveryTimeout = 90 * time.Second

// discoverManifest hỏi Engine tới khi có Manifest hợp lệ, hoặc hết thời gian chờ.
func discoverManifest(ttsClient *client.CoreTTSClient, engineURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for attempt := 1; ; attempt++ {
		manifest, err := ttsClient.GetInfo()
		switch {
		case err != nil:
			lastErr = err
		case manifest == nil:
			lastErr = errors.New("engine trả về manifest rỗng")
		default:
			// Manifest tự mâu thuẫn cũng là chưa sẵn sàng: phục vụ bằng một bản khai mà
			// resolver không diễn giải nổi thì các giới hạn cũng không đáng tin.
			if setErr := state.GlobalManifestState.Set(manifest); setErr != nil {
				lastErr = fmt.Errorf("manifest không hợp lệ: %w", setErr)
				break
			}
			log.Printf("🚀 AI Engine Manifest discovered & cached: %s (v%s) [Max Length: %d chars]",
				manifest.EngineName, manifest.Version, manifest.Constraints.MaxTextLength)
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("sau %s và %d lần thử tại %s: %w", timeout, attempt, engineURL, lastErr)
		}

		log.Printf("⏳ Đang chờ Manifest từ AI Engine tại %s (%v)... thử lại sau 3s", engineURL, lastErr)
		time.Sleep(3 * time.Second)
	}
}

func main() {
	cfg := config.LoadConfig()

	// Create storage directory if missing
	if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage directory '%s': %v", cfg.StorageDir, err)
	}

	// Gốc lưu trữ phải được đặt trước khi bất kỳ handler nào ghi tệp, nếu không chúng sẽ
	// dùng mặc định "storage" và bỏ qua STORAGE_DIR.
	storage.InitStorage(cfg.StorageDir)

	// Trần upload cho mọi handler multipart. Không có dòng này, MAX_UPLOAD_SIZE_MB chỉ là
	// một con số trong log khởi động.
	handlers.SetMaxUploadMB(cfg.MaxUploadMB)

	// Cache người dùng cho tầng xác thực. Không có dòng này thì AUTH_USER_CACHE_SECONDS
	// cũng chỉ là một con số trong log, và mỗi request vẫn hỏi PostgreSQL một lần.
	middleware.ConfigureUserCache(time.Duration(cfg.AuthUserCacheSeconds) * time.Second)

	// Xoá định kỳ các tập tin âm thanh tạm. Mỗi lần tổng hợp ghi một tập tin vào
	// storage/temp và trước đây không có gì dọn chúng, nên đĩa chỉ có thể phình lên.
	storage.StartTempSweeper(
		storage.TempDir(),
		time.Duration(cfg.TempRetentionHours)*time.Hour,
		storage.DefaultSweepInterval,
	)

	// Initialize Database with Connection Pool settings
	db.InitDB(cfg)

	// Initialize Redis Cache & PubSub Broker
	state.InitRedis(cfg)

	// Core TTS client
	ttsClient := client.NewCoreTTSClient(cfg.CoreTTSURL, cfg.TTSClientTimeout)

	// Manifest phải có TRƯỚC khi cổng mở, giống như biến môi trường bắt buộc.
	//
	// Trước đây việc này chạy trong một goroutine nền retry vô hạn, còn server nhận request
	// ngay lập tức. Nên có một cửa sổ — từ lúc khởi động tới khi Engine trả lời, hoặc VĨNH
	// VIỄN nếu Engine không bao giờ lên — mà mọi giới hạn khai trong Manifest đều không có
	// hiệu lực: ValidateRequest/ValidatePitch/ValidateEmotion đều trả nil khi chưa có
	// Manifest, tức max_text_length, speed_range, supported_emotions và cả danh sách mode
	// hợp lệ đều không được áp. Giao diện tự giới hạn 3000 ký tự nên qua UI không thấy gì
	// bất thường; chỉ ai gọi thẳng API mới đi qua được, và đó chính là người ta muốn chặn.
	//
	// Chờ có giới hạn rồi bỏ cuộc, thay vì retry mãi: một backend chạy mà không phục vụ được
	// gì là thứ mọi lớp giám sát đều báo, còn một backend chạy mà không áp giới hạn nào thì
	// trông hoàn toàn khoẻ mạnh.
	if err := discoverManifest(ttsClient, cfg.CoreTTSURL, manifestDiscoveryTimeout); err != nil {
		log.Fatalf("Không lấy được Manifest từ AI Engine: %v.\n"+
			"Backend không khởi động khi thiếu Manifest, vì mọi giới hạn đầu vào (độ dài văn bản,\n"+
			"khoảng speed/pitch, danh sách mode và emotion) đều do Manifest khai. Hãy kiểm tra\n"+
			"CORE_ENGINE_URL và xem Engine đã sẵn sàng chưa.", err)
	}

	// Create Router
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

	// Graceful shutdown channel
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
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited successfully")
}
