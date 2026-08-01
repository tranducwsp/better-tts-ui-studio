package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config chứa toàn bộ thông số cấu hình của hệ thống core-backend, được nạp từ biến môi trường (ENV).
type Config struct {
	Host                     string
	Port                     string
	DatabaseURL              string
	SecretKey                string
	AccessTokenExpireMinutes int
	CoreTTSURL               string
	CoreTTSGrpcURL           string
	FEBuilderURL             string
	DefaultAdminUsername     string
	DefaultAdminPassword     string
	DefaultUserUsername      string
	DefaultUserPassword      string

	// Cấu hình Redis Cache & PubSub Broker
	RedisURL      string
	RedisPassword string

	// Cấu hình Database Connection Pool (pgxpool)
	DBMaxConns                int32
	DBMinConns                int32
	DBMaxConnLifetimeMinutes  int
	DBMaxConnIdleMinutes      int
	DBConnectMaxRetries       int
	DBConnectRetryIntervalSec int

	// Cấu hình Storage & Limit
	StorageDir string
	// Số giờ giữ tập tin âm thanh tạm trước khi bị quét xoá.
	TempRetentionHours int
	MaxUploadMB        int
	CORSOrigins        []string
	TTSClientTimeout   int
}

// resolveSecretKey lấy khoá ký JWT từ ENV, hoặc sinh ngẫu nhiên nếu không được cung cấp.
//
// Trước đây hàm này trả về một chuỗi mặc định cố định nằm sẵn trong mã nguồn. Bất kỳ ai
// đọc được repo đều có thể tự ký một token admin hợp lệ mà không cần mật khẩu — nên một
// giá trị mặc định dùng chung là lỗ hổng, không phải tiện ích.
//
// Khoá ngẫu nhiên khiến mọi phiên đăng nhập mất hiệu lực sau mỗi lần khởi động lại, và
// không dùng được khi chạy nhiều replica. Đó là chủ ý: bất tiện nhưng an toàn, và log đã
// nói rõ cách khắc phục.
func resolveSecretKey() string {
	if v := strings.TrimSpace(os.Getenv("SECRET_KEY")); v != "" {
		return v
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("SECRET_KEY chưa được đặt và không thể sinh khoá ngẫu nhiên: %v", err)
	}
	log.Println("⚠️  SECRET_KEY chưa được đặt. Đã sinh khoá tạm thời: mọi người dùng sẽ bị đăng xuất sau mỗi lần khởi động lại, và nhiều replica sẽ không dùng chung được phiên. Hãy đặt SECRET_KEY cho môi trường thật.")
	return hex.EncodeToString(buf)
}

// LoadConfig đọc file .env và nạp các biến môi trường kèm giá trị fallback an toàn.
func LoadConfig() *Config {
	_ = godotenv.Load()

	host := getEnv("HOST", "0.0.0.0")
	port := getEnv("PORT", "8000")
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/ai_studio?sslmode=disable")
	secretKey := resolveSecretKey()
	coreTTSURL := getEnv("CORE_ENGINE_URL", getEnv("CORE_TTS_URL", "http://localhost:8001"))
	coreTTSGrpcURL := getEnv("CORE_ENGINE_GRPC_URL", getEnv("CORE_TTS_GRPC_URL", "localhost:50051"))
	feBuilderURL := getEnv("FE_BUILDER_URL", "http://frontend-builder:3001")

	redisURL := getEnv("REDIS_URL", "localhost:6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")

	expireMin := getEnvInt("ACCESS_TOKEN_EXPIRE_MINUTES", 10080)

	// Database Connection Pool Configs
	dbMaxConns := int32(getEnvInt("DB_MAX_CONNS", 25))
	dbMinConns := int32(getEnvInt("DB_MIN_CONNS", 5))
	dbMaxConnLifetimeMin := getEnvInt("DB_MAX_CONN_LIFETIME_MINUTES", 30)
	dbMaxConnIdleMin := getEnvInt("DB_MAX_CONN_IDLE_MINUTES", 15)
	dbConnectMaxRetries := getEnvInt("DB_CONNECT_MAX_RETRIES", 10)
	dbConnectRetryIntervalSec := getEnvInt("DB_CONNECT_RETRY_INTERVAL_SECONDS", 2)

	// Storage & App Limits
	storageDir := getEnv("STORAGE_DIR", "storage")
	maxUploadMB := getEnvInt("MAX_UPLOAD_SIZE_MB", 32)
	tempRetentionHours := getEnvInt("TEMP_AUDIO_RETENTION_HOURS", 24)
	corsOriginsStr := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000,http://127.0.0.1:5173")
	corsOrigins := strings.Split(corsOriginsStr, ",")
	for i := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
	}

	ttsTimeout := getEnvInt("TTS_CLIENT_TIMEOUT_SECONDS", 60)

	return &Config{
		Host:                     host,
		Port:                     port,
		DatabaseURL:              dbURL,
		SecretKey:                secretKey,
		AccessTokenExpireMinutes: expireMin,
		CoreTTSURL:               coreTTSURL,
		CoreTTSGrpcURL:           coreTTSGrpcURL,
		FEBuilderURL:             feBuilderURL,
		// Không có mật khẩu mặc định: seedDefaultAccounts bỏ qua tài khoản nào thiếu
		// thông tin, nên không đặt ENV nghĩa là không có tài khoản nào được tạo sẵn.
		DefaultAdminUsername:      getEnv("DEFAULT_ADMIN_USERNAME", ""),
		DefaultAdminPassword:      getEnv("DEFAULT_ADMIN_PASSWORD", ""),
		DefaultUserUsername:       getEnv("DEFAULT_USER_USERNAME", ""),
		DefaultUserPassword:       getEnv("DEFAULT_USER_PASSWORD", ""),
		RedisURL:                  redisURL,
		RedisPassword:             redisPassword,
		DBMaxConns:                dbMaxConns,
		DBMinConns:                dbMinConns,
		DBMaxConnLifetimeMinutes:  dbMaxConnLifetimeMin,
		DBMaxConnIdleMinutes:      dbMaxConnIdleMin,
		DBConnectMaxRetries:       dbConnectMaxRetries,
		DBConnectRetryIntervalSec: dbConnectRetryIntervalSec,
		StorageDir:                storageDir,
		TempRetentionHours:        tempRetentionHours,
		MaxUploadMB:               maxUploadMB,
		CORSOrigins:               corsOrigins,
		TTSClientTimeout:          ttsTimeout,
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if valueStr, exists := os.LookupEnv(key); exists && valueStr != "" {
		if val, err := strconv.Atoi(valueStr); err == nil {
			return val
		}
	}
	return fallback
}
