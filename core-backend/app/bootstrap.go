package app

import (
	"errors"
	"fmt"
	"log"
	"time"

	"core-backend/client"
	"core-backend/config"
	"core-backend/db"
	"core-backend/handlers"
	"core-backend/middleware"
	"core-backend/state"
	"core-backend/storage"
)

// Mode phân biệt hai vai trò mà cùng một binary đảm nhiệm.
type Mode string

const (
	// ModeWeb nhận request và xếp job vào hàng đợi.
	ModeWeb Mode = "web"
	// ModeWorker nhặt job ra chạy, không mở cổng nào.
	ModeWorker Mode = "worker"
)

// ParseMode kiểm giá trị cờ -mode.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case ModeWeb, ModeWorker:
		return Mode(s), nil
	default:
		return "", fmt.Errorf("-mode=%q không hợp lệ; chỉ nhận %q hoặc %q", s, ModeWeb, ModeWorker)
	}
}

// manifestDiscoveryTimeout là thời gian chờ Engine trả về một Manifest hợp lệ lúc khởi động.
//
// Đủ rộng để Engine nạp xong mô hình khi cả hai cùng lên trong một lần `docker compose up`,
// nhưng vẫn hữu hạn: một tiến trình treo mãi ở bước khởi tạo không báo cho ai biết, trong khi
// một tiến trình thoát với thông báo rõ ràng thì mọi orchestrator đều thấy và khởi động lại.
const manifestDiscoveryTimeout = 90 * time.Second

// Bootstrap dựng mọi phụ thuộc dùng chung và trả về client Engine.
//
// Ở đây thay vì trong main vì cả hai chế độ đều cần đúng chuỗi này, theo đúng thứ tự này: kho
// trước khi có ai ghi tệp, DB và Redis trước khi có ai đọc trạng thái, manifest trước khi có
// ai nhận đầu vào. Hai bản chép tay của chuỗi này sẽ trôi khỏi nhau, và cách nó trôi là im
// lặng — một chế độ áp giới hạn mà chế độ kia thì không.
//
// Những gì KHÔNG dùng chung thì nhận mode và tự bỏ qua: worker không xác thực ai và không
// nhận multipart, nên cấu hình những tầng đó ở worker chỉ tạo ấn tượng sai rằng nó có chúng.
func Bootstrap(cfg *config.Config, mode Mode) *client.CoreTTSClient {
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

	if mode == ModeWeb {
		// Trần upload cho mọi handler multipart. Không có dòng này, MAX_UPLOAD_SIZE_MB chỉ là
		// một con số trong log khởi động.
		handlers.SetMaxUploadMB(cfg.MaxUploadMB)

		// Cache người dùng cho tầng xác thực. Không có dòng này thì AUTH_USER_CACHE_SECONDS
		// cũng chỉ là một con số trong log, và mỗi request vẫn hỏi PostgreSQL một lần.
		middleware.ConfigureUserCache(time.Duration(cfg.AuthUserCacheSeconds) * time.Second)

		// Ai được phép đặt X-Forwarded-For. Không có dòng này thì hạn mức khoá theo địa chỉ
		// TCP thật, tức là đúng nhưng gộp mọi người dùng sau một proxy vào chung một khoá.
		middleware.SetTrustedProxies(cfg.TrustedProxies)
	}

	// Xoá định kỳ âm thanh tạm. Mỗi lần tổng hợp ghi một đối tượng vào nhánh temp và trước
	// đây không có gì dọn chúng, nên kho chỉ có thể phình lên.
	//
	// Chỉ chạy ở worker: bộ quét là việc nền, và để nó ở web nghĩa là mỗi replica web thêm
	// một lượt quét toàn bộ nhánh temp mỗi giờ — với S3 thì đó là tiền và hạn mức API, đổi
	// lại không có gì vì các lượt quét xoá đúng cùng một tập tệp.
	if mode == ModeWorker {
		storage.StartTempSweeper(
			storage.Global,
			time.Duration(cfg.TempRetentionHours)*time.Hour,
			storage.DefaultSweepInterval,
		)
	}

	// Kết nối DB. Migration và tài khoản khởi tạo chỉ thuộc về web — xem db.InitOptions.
	db.InitDB(cfg, db.InitOptions{
		Migrate: mode == ModeWeb,
		Seed:    mode == ModeWeb,
	})

	// Redis Cache & PubSub Broker.
	state.InitRedis(cfg)

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
	// Worker cũng cần: nó đọc ResolveAudioSpec để biết Engine trả định dạng gì.
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

	return ttsClient
}

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
