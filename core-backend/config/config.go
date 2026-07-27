package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

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
}

func LoadConfig() *Config {
	_ = godotenv.Load()

	host := getEnv("HOST", "0.0.0.0")
	port := getEnv("PORT", "8000")
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/vieneu_tts?sslmode=disable")
	secretKey := getEnv("SECRET_KEY", "default_secret_key_change_me")
	coreTTSURL := getEnv("CORE_TTS_URL", "http://localhost:8001")

	expireStr := getEnv("ACCESS_TOKEN_EXPIRE_MINUTES", "10080")
	expireMin, err := strconv.Atoi(expireStr)
	if err != nil {
		expireMin = 10080
	}

	return &Config{
		Host:                     host,
		Port:                     port,
		DatabaseURL:              dbURL,
		SecretKey:                secretKey,
		AccessTokenExpireMinutes: expireMin,
		CoreTTSURL:               coreTTSURL,
		DefaultAdminUsername:     os.Getenv("DEFAULT_ADMIN_USERNAME"),
		DefaultAdminPassword:     os.Getenv("DEFAULT_ADMIN_PASSWORD"),
		DefaultUserUsername:      os.Getenv("DEFAULT_USER_USERNAME"),
		DefaultUserPassword:      os.Getenv("DEFAULT_USER_PASSWORD"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return fallback
}
