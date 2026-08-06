package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"core-backend/client"
	"core-backend/config"
	"core-backend/db"
	"core-backend/handlers"
	"core-backend/middleware"
	"core-backend/queue"
	"core-backend/router"
	"core-backend/state"
	"core-backend/storage"
	"core-backend/synth"
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
	// Một binary, hai chế độ. Web nhận request và xếp job; worker nhặt job ra chạy.
	//
	// Cùng binary thay vì hai chương trình riêng: chuỗi khởi tạo bên dưới (kho, DB, Redis,
	// manifest) phải giống hệt nhau ở cả hai, và tách ra hai main là tạo hai bản sao sẽ trôi
	// khỏi nhau. Chúng vẫn scale độc lập được vì là hai service khác nhau trong compose.
	mode := flag.String("mode", "web", `chế độ chạy: "web" hoặc "worker"`)
	flag.Parse()

	if *mode != "web" && *mode != "worker" {
		log.Fatalf("-mode=%q không hợp lệ; chỉ nhận \"web\" hoặc \"worker\"", *mode)
	}

	cfg := config.LoadConfig()

	// Kho lưu trữ phải sẵn sàng trước khi bất kỳ handler nào ghi tệp.
	//
	// Dừng luôn nếu không khởi tạo được, thay vì để handler đầu tiên phát hiện: một backend
	// nhận request mà không cất được âm thanh chỉ tiêu tốn GPU cho những tệp không ai lấy
	// lại được.
	if err := storage.Init(cfg.StorageBackend, cfg.StorageDir, storage.S3Config{
		Bucket:         cfg.S3Bucket,
		Region:         cfg.S3Region,
		Endpoint:       cfg.S3Endpoint,
		AccessKey:      cfg.S3AccessKey,
		SecretKey:      cfg.S3SecretKey,
		ForcePathStyle: cfg.S3ForcePathStyle,
		Prefix:         cfg.S3Prefix,
	}); err != nil {
		log.Fatalf("Không khởi tạo được kho lưu trữ: %v", err)
	}

	// Trần upload cho mọi handler multipart. Không có dòng này, MAX_UPLOAD_SIZE_MB chỉ là
	// một con số trong log khởi động.
	handlers.SetMaxUploadMB(cfg.MaxUploadMB)

	// Cache người dùng cho tầng xác thực. Không có dòng này thì AUTH_USER_CACHE_SECONDS
	// cũng chỉ là một con số trong log, và mỗi request vẫn hỏi PostgreSQL một lần.
	middleware.ConfigureUserCache(time.Duration(cfg.AuthUserCacheSeconds) * time.Second)

	// Ai được phép đặt X-Forwarded-For. Không có dòng này thì hạn mức khoá theo địa chỉ TCP
	// thật, tức là đúng nhưng gộp mọi người dùng sau một proxy vào chung một khoá.
	middleware.SetTrustedProxies(cfg.TrustedProxies)

	// Xoá định kỳ âm thanh tạm. Mỗi lần tổng hợp ghi một đối tượng vào nhánh temp và trước
	// đây không có gì dọn chúng, nên kho chỉ có thể phình lên.
	//
	// Chỉ chạy ở worker: bộ quét là việc nền, và để nó ở web nghĩa là mỗi replica web thêm
	// một lượt quét toàn bộ nhánh temp mỗi giờ — với S3 thì đó là tiền và hạn mức API, đổi
	// lại không có gì vì các lượt quét xoá đúng cùng một tập tệp.
	if *mode == "worker" {
		storage.StartTempSweeper(
			storage.Global,
			time.Duration(cfg.TempRetentionHours)*time.Hour,
			storage.DefaultSweepInterval,
		)
	}

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

	if *mode == "worker" {
		runWorker(cfg, ttsClient)
		return
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
		// Không Fatalf: os.Exit bỏ qua mọi defer, và ngay dưới đây còn phải chờ các lượt tổng
		// hợp chạy tại chỗ. Hết giờ đóng listener không phải lý do để vứt công việc đang chạy.
		log.Printf("Server chưa đóng gọn trong hạn: %v", err)
	}

	// Chờ nhánh dự phòng không-Redis chạy nốt. Shutdown ở trên chỉ chờ kết nối HTTP, mà lượt
	// tổng hợp đã trả task_id về từ lâu nên nó không nhìn thấy — thoát luôn ở đây sẽ để chunk
	// mắc lại "processing" và người dùng thấy một job không bao giờ xong.
	synth.WaitLocal(30 * time.Second)

	log.Println("Server exited successfully")
}

// runWorker nhặt job từ hàng đợi và chạy cho tới khi nhận tín hiệu dừng.
//
// Không mở cổng nào: worker không phục vụ request, và mở một listener chỉ để healthcheck sẽ
// tạo ra một bề mặt không ai dùng. Trạng thái của nó nhìn được qua log và qua chính hàng đợi.
func runWorker(cfg *config.Config, ttsClient *client.CoreTTSClient) {
	if state.RedisClient == nil {
		log.Fatal("Worker cần Redis để nhận job, nhưng REDIS_URL chưa cấu hình hoặc không kết nối được.\n" +
			"Không có Redis thì chạy chế độ web là đủ: nó tự tổng hợp tại chỗ.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := queue.EnsureGroup(ctx); err != nil {
		log.Fatalf("Không tạo được nhóm tiêu thụ hàng đợi: %v", err)
	}

	// Tên định danh worker trong nhóm. Hostname là tên container trong compose và tên pod
	// trên k8s, nên nó vừa duy nhất vừa truy ngược được về tiến trình thật.
	name, err := os.Hostname()
	if err != nil || name == "" {
		name = "worker"
	}

	// Trần số job chạy song song trong MỘT worker.
	//
	// Engine là nút cổ chai: nó bám GPU, nên đẩy nhiều lượt hơn số nó xử được chỉ làm mọi
	// lượt chậm đi. Muốn nhiều hơn thì chạy thêm worker — đó là lý do tách tiến trình.
	const maxInFlight = 2
	slots := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup

	log.Printf("Worker %q sẵn sàng, tối đa %d job song song", name, maxInFlight)

	err = queue.Consume(ctx, name, func(jobCtx context.Context, job queue.Job) {
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
