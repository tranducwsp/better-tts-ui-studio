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

	expireMin := getEnvInt("ACCESS_TOKEN_EXPIRE_MINUTES", 10080, 1, 525600)

	// Database Connection Pool Configs
	dbMaxConns := int32(getEnvInt("DB_MAX_CONNS", 25, 1, 10000))
	dbMinConns := int32(getEnvInt("DB_MIN_CONNS", 5, 0, 10000))
	dbMaxConnLifetimeMin := getEnvInt("DB_MAX_CONN_LIFETIME_MINUTES", 30, 1, 10080)
	dbMaxConnIdleMin := getEnvInt("DB_MAX_CONN_IDLE_MINUTES", 15, 1, 10080)
	dbConnectMaxRetries := getEnvInt("DB_CONNECT_MAX_RETRIES", 10, 1, 1000)
	dbConnectRetryIntervalSec := getEnvInt("DB_CONNECT_RETRY_INTERVAL_SECONDS", 2, 1, 3600)

	// Storage & App Limits
	storageDir := getEnv("STORAGE_DIR", "storage")
	maxUploadMB := getEnvInt("MAX_UPLOAD_SIZE_MB", 32, 1, 10240)
	tempRetentionHours := getEnvInt("TEMP_AUDIO_RETENTION_HOURS", 24, 1, 8760)
	corsOriginsStr := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000,http://127.0.0.1:5173")
	corsOrigins := strings.Split(corsOriginsStr, ",")
	for i := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
	}

	ttsTimeout := getEnvInt("TTS_CLIENT_TIMEOUT_SECONDS", 60, 1, 3600)

	// Ràng buộc chéo: pgxpool sẽ báo lỗi khi khởi tạo nếu MinConns lớn hơn MaxConns, nhưng
	// thông báo của nó không chỉ ra biến môi trường nào cần sửa.
	if dbMinConns > dbMaxConns {
		log.Fatalf("DB_MIN_CONNS (%d) không được lớn hơn DB_MAX_CONNS (%d).", dbMinConns, dbMaxConns)
	}

	cfg := &Config{
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

	cfg.logSummary()
	return cfg
}

// logSummary in ra cấu hình đang có hiệu lực ngay khi khởi động.
//
// Không có nó, một biến gõ sai tên (DB_MAXCONNS thay vì DB_MAX_CONNS) sẽ im lặng dùng mặc
// định và người vận hành không có cách nào đối chiếu ý định với thực tế. Bí mật chỉ hiện
// trạng thái, không hiện giá trị.
func (c *Config) logSummary() {
	secretState := "đã đặt qua ENV"
	if strings.TrimSpace(os.Getenv("SECRET_KEY")) == "" {
		secretState = "SINH NGẪU NHIÊN (phiên đăng nhập mất sau mỗi lần khởi động lại)"
	}

	seeded := "không tạo tài khoản nào"
	if c.DefaultAdminUsername != "" || c.DefaultUserUsername != "" {
		seeded = "có tài khoản khởi tạo sẵn"
	}

	log.Printf("Cấu hình đang áp dụng:")
	log.Printf("  server        %s:%s | token hết hạn sau %d phút", c.Host, c.Port, c.AccessTokenExpireMinutes)
	log.Printf("  secret        %s | %s", secretState, seeded)
	log.Printf("  engine        %s (timeout %ds)", c.CoreTTSURL, c.TTSClientTimeout)
	log.Printf("  db pool       max=%d min=%d lifetime=%dm idle=%dm retry=%dx%ds",
		c.DBMaxConns, c.DBMinConns, c.DBMaxConnLifetimeMinutes, c.DBMaxConnIdleMinutes,
		c.DBConnectMaxRetries, c.DBConnectRetryIntervalSec)
	log.Printf("  storage       %s | giữ tạm %dh | upload tối đa %dMB",
		c.StorageDir, c.TempRetentionHours, c.MaxUploadMB)
	log.Printf("  cors          %s", strings.Join(c.CORSOrigins, ", "))
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return fallback
}

// getEnvInt đọc một biến môi trường kiểu số nguyên.
//
// Giá trị không phân tích được sẽ làm tiến trình dừng lại, thay vì lặng lẽ quay về mặc
// định. Một biến gõ sai — "twenty", "32MB", "10 " — trước đây bị bỏ qua không dấu vết, nên
// người vận hành tin rằng cấu hình đã có hiệu lực trong khi hệ thống chạy bằng giá trị
// khác. Hỏng ngay lúc khởi động dễ sửa hơn nhiều so với hỏng ngầm.
//
// min và max là khoảng đóng cho phép; truyền math.MinInt/math.MaxInt nếu không cần chặn.
func getEnvInt(key string, fallback, min, max int) int {
	raw, exists := os.LookupEnv(key)
	if !exists || strings.TrimSpace(raw) == "" {
		return fallback
	}

	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		log.Fatalf("Biến môi trường %s = %q không phải số nguyên hợp lệ. Hãy sửa hoặc bỏ biến này để dùng mặc định (%d).", key, raw, fallback)
	}
	if val < min || val > max {
		log.Fatalf("Biến môi trường %s = %d nằm ngoài khoảng cho phép (%d đến %d).", key, val, min, max)
	}
	return val
}
