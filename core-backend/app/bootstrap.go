package app

import (
	"errors"
	"fmt"
	"log"
	"time"

	"core-backend/audio"
	"core-backend/client"
	"core-backend/config"
	"core-backend/db"
	"core-backend/handlers"
	"core-backend/middleware"
	"core-backend/state"
	"core-backend/storage"
)

// BootstrapWeb khởi tạo mọi thứ mà HTTP backend cần rồi trả về client Engine.
//
// Web là tiến trình duy nhất chạy migration, seed tài khoản và cấu hình auth middleware.
// Worker dùng BootstrapWorker; cron dùng BootstrapCron — không có mode flag hay nhánh ẩn để
// đoán một tiến trình đang làm vai trò nào.
func BootstrapWeb(cfg *config.Config) *client.CoreTTSClient {
	initStorage(cfg)

	handlers.SetMaxUploadMB(cfg.MaxUploadMB)
	audio.ConfigureTranscoding(time.Duration(cfg.TranscodeTimeoutSeconds)*time.Second, cfg.TranscodeMaxConcurrency)
	middleware.ConfigureUserCache(time.Duration(cfg.AuthUserCacheSeconds) * time.Second)
	middleware.SetTrustedProxies(cfg.TrustedProxies)

	return bootstrapEngine(cfg, db.InitOptions{Migrate: true, Seed: true})
}

// BootstrapWorker khởi tạo những phụ thuộc worker cần để nhặt job và ghi kết quả.
//
// Worker không chạy migration/seed, không cấu hình auth và không nhận multipart request. DB
// vẫn cần cho UpdateChunkStatus; Redis cần cho Stream và trạng thái liên tiến trình.
func BootstrapWorker(cfg *config.Config) *client.CoreTTSClient {
	initStorage(cfg)
	return bootstrapEngine(cfg, db.InitOptions{})
}

// BootstrapCron chỉ khởi tạo storage rồi bắt đầu sweeper.
//
// Cron không cần DB, Redis hay Engine: nó chỉ LIST/DELETE các object temp. Giữ phụ thuộc của
// nó nhỏ và rõ để một lỗi ở database không làm chết một bộ dọn rác không liên quan.
func BootstrapCron(cfg *config.Config) {
	initStorage(cfg)
	storage.StartTempSweeper(
		storage.Global,
		time.Duration(cfg.TempRetentionHours)*time.Hour,
		storage.DefaultSweepInterval,
	)
}

func initStorage(cfg *config.Config) {
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
}

func bootstrapEngine(cfg *config.Config, dbOptions db.InitOptions) *client.CoreTTSClient {
	db.InitDB(cfg, dbOptions)
	state.InitRedis(cfg)

	ttsClient := client.NewCoreTTSClient(cfg.CoreTTSURL, cfg.TTSClientTimeout)
	if err := discoverManifest(ttsClient, cfg.CoreTTSURL, 90*time.Second, 3*time.Second); err != nil {
		log.Fatalf("Không lấy được Manifest từ AI Engine: %v.\n"+
			"Backend không khởi động khi thiếu Manifest, vì mọi giới hạn đầu vào (độ dài văn bản,\n"+
			"khoảng speed/pitch, danh sách mode và emotion) đều do Manifest khai. Hãy kiểm tra\n"+
			"CORE_ENGINE_URL và xem Engine đã sẵn sàng chưa.", err)
	}
	return ttsClient
}

// discoverManifest hỏi Engine tới khi có Manifest hợp lệ, hoặc hết thời gian chờ.
func discoverManifest(ttsClient *client.CoreTTSClient, engineURL string, timeout, retryInterval time.Duration) error {
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
		time.Sleep(retryInterval)
	}
}
