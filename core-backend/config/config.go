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

// LoadConfig đọc .env rồi nạp mọi biến theo bảng đặc tả trong settings.go.
//
// Không giá trị mặc định nào xuất hiện ở đây — chúng chỉ tồn tại trong Settings, để một
// người muốn biết "biến X mặc định là gì" chỉ phải đọc một bảng thay vì lần theo mã khởi
// tạo.
func LoadConfig() *Config {
	_ = godotenv.Load()

	corsOrigins := strings.Split(str("CORS_ALLOWED_ORIGINS"), ",")
	for i := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
	}

	dbMaxConns := int32(num("DB_MAX_CONNS"))
	dbMinConns := int32(num("DB_MIN_CONNS"))

	// Ràng buộc chéo: pgxpool báo lỗi khi MinConns lớn hơn MaxConns, nhưng thông báo của nó
	// không chỉ ra biến môi trường nào cần sửa.
	if dbMinConns > dbMaxConns {
		log.Fatalf("DB_MIN_CONNS (%d) không được lớn hơn DB_MAX_CONNS (%d).", dbMinConns, dbMaxConns)
	}

	cfg := &Config{
		Host:                     str("HOST"),
		Port:                     str("PORT"),
		DatabaseURL:              str("DATABASE_URL"),
		SecretKey:                resolveSecretKey(),
		AccessTokenExpireMinutes: num("ACCESS_TOKEN_EXPIRE_MINUTES"),
		CoreTTSURL:               str("CORE_ENGINE_URL"),
		CoreTTSGrpcURL:           str("CORE_ENGINE_GRPC_URL"),
		FEBuilderURL:             str("FE_BUILDER_URL"),

		DefaultAdminUsername: str("DEFAULT_ADMIN_USERNAME"),
		DefaultAdminPassword: str("DEFAULT_ADMIN_PASSWORD"),
		DefaultUserUsername:  str("DEFAULT_USER_USERNAME"),
		DefaultUserPassword:  str("DEFAULT_USER_PASSWORD"),

		RedisURL:      str("REDIS_URL"),
		RedisPassword: str("REDIS_PASSWORD"),

		DBMaxConns:                dbMaxConns,
		DBMinConns:                dbMinConns,
		DBMaxConnLifetimeMinutes:  num("DB_MAX_CONN_LIFETIME_MINUTES"),
		DBMaxConnIdleMinutes:      num("DB_MAX_CONN_IDLE_MINUTES"),
		DBConnectMaxRetries:       num("DB_CONNECT_MAX_RETRIES"),
		DBConnectRetryIntervalSec: num("DB_CONNECT_RETRY_INTERVAL_SECONDS"),

		StorageDir:         str("STORAGE_DIR"),
		TempRetentionHours: num("TEMP_AUDIO_RETENTION_HOURS"),
		MaxUploadMB:        num("MAX_UPLOAD_SIZE_MB"),
		CORSOrigins:        corsOrigins,
		TTSClientTimeout:   num("TTS_CLIENT_TIMEOUT_SECONDS"),
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

// str đọc một biến chuỗi theo đặc tả, thử các tên cũ (Aliases) trước khi dùng mặc định.
func str(key string) string {
	s := lookup(key)
	if v, ok := firstSet(append([]string{s.Key}, s.Aliases...)); ok {
		return v
	}
	return s.Default
}

// num đọc một biến số nguyên theo đặc tả.
//
// Giá trị không phân tích được sẽ làm tiến trình dừng lại, thay vì lặng lẽ quay về mặc
// định. Một biến gõ sai — "twenty", "32MB" — trước đây bị bỏ qua không dấu vết, nên người
// vận hành tin rằng cấu hình đã có hiệu lực trong khi hệ thống chạy bằng giá trị khác.
// Hỏng ngay lúc khởi động dễ sửa hơn nhiều so với hỏng ngầm.
func num(key string) int {
	s := lookup(key)

	raw, ok := firstSet(append([]string{s.Key}, s.Aliases...))
	if !ok {
		raw = s.Default
	}

	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		log.Fatalf("Biến môi trường %s = %q không phải số nguyên hợp lệ. Hãy sửa hoặc bỏ biến này để dùng mặc định (%s).", s.Key, raw, s.Default)
	}
	if val < s.Min || val > s.Max {
		log.Fatalf("Biến môi trường %s = %d nằm ngoài khoảng cho phép (%d đến %d).", s.Key, val, s.Min, s.Max)
	}
	return val
}

// firstSet trả về giá trị không rỗng đầu tiên trong danh sách tên.
func firstSet(keys []string) (string, bool) {
	for _, k := range keys {
		if v, exists := os.LookupEnv(k); exists && strings.TrimSpace(v) != "" {
			return v, true
		}
	}
	return "", false
}
