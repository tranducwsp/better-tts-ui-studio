package config

import (
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
	DefaultAdminUsername     string
	DefaultAdminPassword     string
	DefaultUserUsername      string
	DefaultUserPassword      string

	// Cấu hình Database Connection Pool (pgxpool)
	DBMaxConns                int32
	DBMinConns                int32
	DBMaxConnLifetimeMinutes  int
	DBMaxConnIdleMinutes      int
	DBConnectMaxRetries       int
	DBConnectRetryIntervalSec int

	// Cấu hình Storage & Limit
	StorageDir      string
	MaxUploadMB     int
	CORSOrigins     []string
	TTSClientTimeout int
}

// LoadConfig đọc file .env và nạp các biến môi trường kèm giá trị fallback an toàn.
func LoadConfig() *Config {
	_ = godotenv.Load()

	host := getEnv("HOST", "0.0.0.0")
	port := getEnv("PORT", "8000")
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/vieneu_tts?sslmode=disable")
	secretKey := getEnv("SECRET_KEY", "default_secret_key_change_me")
	coreTTSURL := getEnv("CORE_TTS_URL", "http://localhost:8001")

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
	corsOriginsStr := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000,http://127.0.0.1:5173")
	corsOrigins := strings.Split(corsOriginsStr, ",")
	for i := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
	}

	ttsTimeout := getEnvInt("TTS_CLIENT_TIMEOUT_SECONDS", 60)

	return &Config{
		Host:                      host,
		Port:                      port,
		DatabaseURL:               dbURL,
		SecretKey:                 secretKey,
		AccessTokenExpireMinutes:  expireMin,
		CoreTTSURL:                coreTTSURL,
		DefaultAdminUsername:      os.Getenv("DEFAULT_ADMIN_USERNAME"),
		DefaultAdminPassword:      os.Getenv("DEFAULT_ADMIN_PASSWORD"),
		DefaultUserUsername:       os.Getenv("DEFAULT_USER_USERNAME"),
		DefaultUserPassword:       os.Getenv("DEFAULT_USER_PASSWORD"),
		DBMaxConns:                dbMaxConns,
		DBMinConns:                dbMinConns,
		DBMaxConnLifetimeMinutes:  dbMaxConnLifetimeMin,
		DBMaxConnIdleMinutes:      dbMaxConnIdleMin,
		DBConnectMaxRetries:       dbConnectMaxRetries,
		DBConnectRetryIntervalSec: dbConnectRetryIntervalSec,
		StorageDir:                storageDir,
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
