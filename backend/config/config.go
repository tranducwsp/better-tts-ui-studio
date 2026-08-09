package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config chứa toàn bộ thông số cấu hình của hệ thống backend, được nạp từ biến môi trường (ENV).
type Config struct {
	Host                      string
	Port                      string
	DatabaseURL               string
	SecretKey                 string
	AccessTokenExpireMinutes  int
	RefreshTokenExpireMinutes int
	// Số giây ghi nhớ bản ghi người dùng sau khi xác thực token. 0 nghĩa là không cache.
	AuthUserCacheSeconds int
	CoreTTSURL           string
	CoreTTSGrpcURL       string
	FEBuilderURL         string
	DefaultAdminUsername string
	DefaultAdminPassword string
	DefaultUserUsername  string
	DefaultUserPassword  string

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
	StorageBackend string
	StorageDir     string

	// Cấu hình kho S3, chỉ có nghĩa khi StorageBackend là "s3".
	S3Bucket         string
	S3Region         string
	S3Endpoint       string
	S3AccessKey      string
	S3SecretKey      string
	S3ForcePathStyle bool
	S3Prefix         string
	// Số giờ giữ tập tin âm thanh tạm trước khi bị quét xoá.
	TempRetentionHours         int
	MaxUploadMB                int
	// Giữ lại file tham chiếu khi xoá giọng clone (thay vì xoá file vật lý).
	PreserveFiles              bool
	CORSOrigins                []string
	CookieSecure               bool
	TTSClientTimeout           int
	WorkerMaxInFlight          int
	TranscodeMaxConcurrency    int
	TranscodeTimeoutSeconds    int
	AuthRateLimitRequests      int
	AuthRateLimitWindowSeconds int
	StaleChunkAfterMinutes     int

	// TrustedProxies là các dải CIDR được phép đặt X-Forwarded-For.
	//
	// Rỗng nghĩa là không tin header đó: hạn mức khoá theo địa chỉ TCP thật. Xem
	// middleware.SetTrustedProxies.
	TrustedProxies []string
}

// requireAll dừng tiến trình nếu bất kỳ biến nào đánh dấu Required bị bỏ trống.
//
// Trước đây cờ Required chỉ là trang trí: gen-env đọc nó để in dòng "BẮT BUỘC cho môi
// trường thật" vào .env.example, còn lúc chạy thì không ai kiểm. Nên một triển khai thiếu
// SECRET_KEY vẫn khởi động bình thường bằng khoá ngẫu nhiên, và người vận hành chỉ biết nếu
// tình cờ đọc log — đúng kiểu hỏng ngầm mà bảng đặc tả này sinh ra để tránh.
//
// Gom mọi biến thiếu rồi báo một lần: sửa một biến, khởi động lại, phát hiện thiếu biến
// tiếp theo là vòng lặp không cần thiết khi tất cả đều biết được ngay từ đầu.
func requireAll() {
	var missing []string
	for _, s := range Settings {
		if !s.Required {
			continue
		}
		// Biến của dịch vụ khác không phải việc của tiến trình này. POSTGRES_PASSWORD chẳng
		// hạn: docker-compose tự bắt buộc nó qua cú pháp `:?`, còn backend chỉ thấy
		// DATABASE_URL đã dựng sẵn — nên đòi nó ở đây sẽ chặn khởi động vô cớ ở mọi triển
		// khai không dùng compose, ví dụ k8s cấp thẳng DATABASE_URL.
		if s.ReadBy != "" {
			continue
		}
		if strings.TrimSpace(os.Getenv(s.Key)) == "" {
			missing = append(missing, s.Key)
		}
	}

	if len(missing) > 0 {
		log.Fatalf("Thiếu biến môi trường bắt buộc: %s.\n"+
			"Sao chép .env.example thành .env rồi điền chúng. Sinh giá trị ngẫu nhiên bằng:\n"+
			"  openssl rand -hex 32",
			strings.Join(missing, ", "))
	}
}

// resolveSecretKey lấy khoá ký JWT từ ENV.
//
// Trước đây hàm này trả về một chuỗi mặc định cố định nằm sẵn trong mã nguồn. Bất kỳ ai
// đọc được repo đều có thể tự ký một token admin hợp lệ mà không cần mật khẩu — nên một
// giá trị mặc định dùng chung là lỗ hổng, không phải tiện ích.
//
// Bước sau đó là sinh khoá ngẫu nhiên khi ENV bỏ trống. An toàn hơn giá trị cứng, nhưng vẫn
// để hệ thống khởi động ở một trạng thái không ai chọn: phiên mất sau mỗi lần khởi động lại
// và nhiều replica không dùng chung được phiên, chỉ báo bằng một dòng log dễ trôi. requireAll
// đã chặn từ trước, nên tới đây khoá chắc chắn có.
func resolveSecretKey() string {
	return strings.TrimSpace(os.Getenv("SECRET_KEY"))
}

// LoadConfig đọc .env rồi nạp mọi biến theo bảng đặc tả trong settings.go.
//
// Không giá trị mặc định nào xuất hiện ở đây — chúng chỉ tồn tại trong Settings, để một
// người muốn biết "biến X mặc định là gì" chỉ phải đọc một bảng thay vì lần theo mã khởi
// tạo.
func LoadConfig() *Config {
	_ = godotenv.Load()

	// Trước mọi thứ khác: thiếu một biến bắt buộc thì không có cấu hình nào để nói tới.
	requireAll()

	corsOrigins := strings.Split(str("CORS_ALLOWED_ORIGINS"), ",")
	for i := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
	}

	// Bỏ phần tử rỗng: mặc định của TRUSTED_PROXIES là chuỗi rỗng, mà Split trả về một lát
	// cắt một phần tử rỗng chứ không phải lát cắt rỗng — và một mục rỗng sẽ không phân tích
	// được thành CIDR nào.
	var trustedProxies []string
	for _, p := range strings.Split(str("TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			trustedProxies = append(trustedProxies, p)
		}
	}

	dbMaxConns := int32(num("DB_MAX_CONNS"))
	dbMinConns := int32(num("DB_MIN_CONNS"))

	// Ràng buộc chéo: pgxpool báo lỗi khi MinConns lớn hơn MaxConns, nhưng thông báo của nó
	// không chỉ ra biến môi trường nào cần sửa.
	if dbMinConns > dbMaxConns {
		log.Fatalf("DB_MIN_CONNS (%d) không được lớn hơn DB_MAX_CONNS (%d).", dbMinConns, dbMaxConns)
	}

	cfg := &Config{
		Host:                      str("HOST"),
		Port:                      str("PORT"),
		DatabaseURL:               str("DATABASE_URL"),
		SecretKey:                 resolveSecretKey(),
		AccessTokenExpireMinutes:  num("ACCESS_TOKEN_EXPIRE_MINUTES"),
		RefreshTokenExpireMinutes: num("REFRESH_TOKEN_EXPIRE_MINUTES"),
		AuthUserCacheSeconds:      num("AUTH_USER_CACHE_SECONDS"),
		CoreTTSURL:                str("CORE_ENGINE_URL"),
		CoreTTSGrpcURL:            str("CORE_ENGINE_GRPC_URL"),
		FEBuilderURL:              str("FE_BUILDER_URL"),

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

		StorageBackend: str("STORAGE_BACKEND"),
		StorageDir:     str("STORAGE_DIR"),

		S3Bucket:                   str("S3_BUCKET"),
		S3Region:                   str("S3_REGION"),
		S3Endpoint:                 str("S3_ENDPOINT"),
		S3AccessKey:                str("S3_ACCESS_KEY_ID"),
		S3SecretKey:                str("S3_SECRET_ACCESS_KEY"),
		S3ForcePathStyle:           num("S3_FORCE_PATH_STYLE") == 1,
		S3Prefix:                   str("S3_PREFIX"),
		TempRetentionHours:         num("TEMP_AUDIO_RETENTION_HOURS"),
		MaxUploadMB:                num("MAX_UPLOAD_SIZE_MB"),
			PreserveFiles:              num("PRESERVE_FILES") == 1,
		CORSOrigins:                corsOrigins,
		CookieSecure:               num("COOKIE_SECURE") == 1,
		TTSClientTimeout:           num("TTS_CLIENT_TIMEOUT_SECONDS"),
		WorkerMaxInFlight:          num("WORKER_MAX_IN_FLIGHT"),
		TranscodeMaxConcurrency:    num("TRANSCODE_MAX_CONCURRENCY"),
		TranscodeTimeoutSeconds:    num("TRANSCODE_TIMEOUT_SECONDS"),
		AuthRateLimitRequests:      num("AUTH_RATE_LIMIT_REQUESTS"),
		AuthRateLimitWindowSeconds: num("AUTH_RATE_LIMIT_WINDOW_SECONDS"),
		StaleChunkAfterMinutes:     num("STALE_CHUNK_AFTER_MINUTES"),
		TrustedProxies:             trustedProxies,
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
	seeded := "không tạo tài khoản nào"
	if c.DefaultAdminUsername != "" || c.DefaultUserUsername != "" {
		seeded = "có tài khoản khởi tạo sẵn"
	}

	log.Printf("Cấu hình đang áp dụng:")
	log.Printf("  server        %s:%s | token hết hạn sau %d phút", c.Host, c.Port, c.AccessTokenExpireMinutes)
	log.Printf("  secret        đã đặt qua ENV | %s", seeded)
	log.Printf("  engine        %s (timeout %ds) | %s", c.CoreTTSURL, c.TTSClientTimeout, c.grpcStatus())
	log.Printf("  db pool       max=%d min=%d lifetime=%dm idle=%dm retry=%dx%ds",
		c.DBMaxConns, c.DBMinConns, c.DBMaxConnLifetimeMinutes, c.DBMaxConnIdleMinutes,
		c.DBConnectMaxRetries, c.DBConnectRetryIntervalSec)
	log.Printf("  storage       backend=%s dir=%s | giữ tạm %dh | upload tối đa %dMB",
		c.StorageBackend, c.StorageDir, c.TempRetentionHours, c.MaxUploadMB)
	log.Printf("  cors          %s", strings.Join(c.CORSOrigins, ", "))
}

func (c *Config) grpcStatus() string {
	if c.CoreTTSGrpcURL != "" {
		return "gRPC + HTTP (gRPC cho Synthesize, HTTP cho Manifest/Voices/Clone)"
	}
	return "HTTP only"
}

// str đọc một biến chuỗi theo đặc tả.
func str(key string) string {
	s := lookup(key)
	if v, exists := os.LookupEnv(s.Key); exists && strings.TrimSpace(v) != "" {
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

	raw, exists := os.LookupEnv(s.Key)
	if !exists || strings.TrimSpace(raw) == "" {
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
