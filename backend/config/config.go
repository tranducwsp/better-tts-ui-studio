package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all backend system configuration parameters, loaded from environment variables (ENV).
type Config struct {
	Host                      string
	Port                      string
	DatabaseURL               string
	SecretKey                 string
	AccessTokenExpireMinutes  int
	RefreshTokenExpireMinutes int
	// Number of seconds to cache the user record after token authentication. 0 means no caching.
	AuthUserCacheSeconds       int
	TaskMemoryRetentionSeconds int
	EnableRequestLogging       bool
	EnablePprof                bool
	EnableSwagger              bool
	CoreTTSURL                 string
	CoreTTSGrpcURL             string
	FEBuilderURL               string
	DefaultAdminUsername       string
	DefaultAdminPassword       string

	// Redis Cache & PubSub Broker configuration
	RedisURL      string
	RedisPassword string

	// Database Connection Pool configuration (pgxpool)
	DBMaxConns                int32
	DBMinConns                int32
	DBMaxConnLifetimeMinutes  int
	DBMaxConnIdleMinutes      int
	DBConnectMaxRetries       int
	DBConnectRetryIntervalSec int

	// Storage & Limit configuration
	StorageBackend string
	StorageDir     string

	// S3 storage configuration, only meaningful when StorageBackend is "s3".
	S3Bucket         string
	S3Region         string
	S3Endpoint       string
	S3AccessKey      string
	S3SecretKey      string
	S3ForcePathStyle bool
	S3Prefix         string
	// Number of hours to keep temporary audio files before they are swept and deleted.
	TempRetentionHours int
	MaxUploadMB        int
	// Keep reference files when deleting cloned voices (instead of physically deleting them).
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

	// TrustedProxies are the CIDR ranges allowed to set X-Forwarded-For.
	//
	// Empty means the header is not trusted: rate limits are keyed by the real TCP address. See
	// middleware.SetTrustedProxies.
	TrustedProxies []string
}

// requireAll stops the process if any variable marked Required is left empty.
//
// Previously the Required flag was only decorative: gen-env read it to print a "REQUIRED for
// real environments" line in .env.example, but at runtime no one checked. So a deployment
// missing SECRET_KEY would still start normally with a random key, and the operator would only
// know if they happened to read the log — exactly the kind of silent failure this spec table
// was designed to prevent.
//
// Collect all missing variables and report them once: fixing one, restarting, discovering the
// next missing variable is an unnecessary loop when all can be known upfront.
func requireAll() {
	var missing []string
	for _, s := range Settings {
		if !s.Required {
			continue
		}
		// Variables belonging to other services are not this process's concern. POSTGRES_PASSWORD
		// for example: docker-compose enforces it via `:?` syntax, while the backend only sees
		// DATABASE_URL already constructed — requiring it here would block startup needlessly in
		// every deployment that doesn't use compose, e.g. k8s providing DATABASE_URL directly.
		if s.ReadBy != "" {
			continue
		}
		if strings.TrimSpace(os.Getenv(s.Key)) == "" {
			missing = append(missing, s.Key)
		}
	}

	if len(missing) > 0 {
		log.Fatalf("Missing required environment variables: %s.\n"+
			"Copy .env.example to .env and fill them in. Generate random values with:\n"+
			"  openssl rand -hex 32",
			strings.Join(missing, ", "))
	}
}

// resolveSecretKey retrieves the JWT signing key from ENV.
//
// Previously this function returned a hardcoded default string baked into the source. Anyone
// who could read the repo could self-sign a valid admin token without a password — so a shared
// default value is a vulnerability, not a convenience.
//
// The next step was generating a random key when ENV was empty. Safer than a hardcoded value,
// but still let the system start in a state no one chose: sessions lost after every restart
// and multiple replicas could not share sessions, signaled only by an easily-missed log line.
// requireAll already blocks this upfront, so by the time we get here the key is guaranteed
// present.
func resolveSecretKey() string {
	return strings.TrimSpace(os.Getenv("SECRET_KEY"))
}

// LoadConfig reads .env and loads all variables according to the specification table in
// settings.go.
//
// No default values appear here — they only exist in Settings, so someone wanting to know
// "what is the default for variable X" only has to read one table instead of tracing through
// initialization code.
func LoadConfig() *Config {
	_ = godotenv.Load()

	// Before anything else: a missing required variable means there is no configuration to speak of.
	requireAll()

	corsOrigins := strings.Split(str("CORS_ALLOWED_ORIGINS"), ",")
	for i := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
	}

	// Remove empty elements: TRUSTED_PROXIES defaults to an empty string, and Split returns a
	// single-element slice containing an empty string rather than an empty slice — and an empty
	// entry cannot be parsed as any CIDR.
	var trustedProxies []string
	for _, p := range strings.Split(str("TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			trustedProxies = append(trustedProxies, p)
		}
	}

	dbMaxConns := int32(num("DB_MAX_CONNS"))
	dbMinConns := int32(num("DB_MIN_CONNS"))

	// Cross constraint: pgxpool reports an error when MinConns exceeds MaxConns, but its message
	// does not indicate which environment variable to fix.
	if dbMinConns > dbMaxConns {
		log.Fatalf("DB_MIN_CONNS (%d) must not be greater than DB_MAX_CONNS (%d).", dbMinConns, dbMaxConns)
	}

	cfg := &Config{
		Host:                       str("HOST"),
		Port:                       str("PORT"),
		DatabaseURL:                str("DATABASE_URL"),
		SecretKey:                  resolveSecretKey(),
		AccessTokenExpireMinutes:   num("ACCESS_TOKEN_EXPIRE_MINUTES"),
		RefreshTokenExpireMinutes:  num("REFRESH_TOKEN_EXPIRE_MINUTES"),
		AuthUserCacheSeconds:       num("AUTH_USER_CACHE_SECONDS"),
		TaskMemoryRetentionSeconds: num("TASK_MEMORY_RETENTION_SECONDS"),
		EnableRequestLogging:       strings.ToLower(str("ENABLE_REQUEST_LOGGING")) != "false" && str("ENABLE_REQUEST_LOGGING") != "0",
		EnablePprof:                strings.ToLower(str("ENABLE_PPROF")) == "true" || str("ENABLE_PPROF") == "1",
		EnableSwagger:              strings.ToLower(str("ENABLE_SWAGGER")) != "false" && str("ENABLE_SWAGGER") != "0",
		CoreTTSURL:                 str("CORE_ENGINE_URL"),
		CoreTTSGrpcURL:             str("CORE_ENGINE_GRPC_URL"),
		FEBuilderURL:               str("FE_BUILDER_URL"),

		DefaultAdminUsername: str("DEFAULT_ADMIN_USERNAME"),
		DefaultAdminPassword: str("DEFAULT_ADMIN_PASSWORD"),

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

// logSummary prints the configuration currently in effect at startup.
//
// Without it, a mistyped variable name (DB_MAXCONNS instead of DB_MAX_CONNS) would silently
// use the default and the operator would have no way to compare intent against reality. Secrets
// only show their presence, not their value.
func (c *Config) logSummary() {
	seeded := "no accounts seeded"
	if c.DefaultAdminUsername != "" {
		seeded = "accounts will be seeded"
	}

	log.Printf("Active configuration:")
	log.Printf("  server        %s:%s | token expires in %d min", c.Host, c.Port, c.AccessTokenExpireMinutes)
	log.Printf("  secret        set via ENV | %s", seeded)
	log.Printf("  engine        %s (timeout %ds) | %s", c.CoreTTSURL, c.TTSClientTimeout, c.grpcStatus())
	log.Printf("  db pool       max=%d min=%d lifetime=%dm idle=%dm retry=%dx%ds",
		c.DBMaxConns, c.DBMinConns, c.DBMaxConnLifetimeMinutes, c.DBMaxConnIdleMinutes,
		c.DBConnectMaxRetries, c.DBConnectRetryIntervalSec)
	log.Printf("  storage       backend=%s dir=%s | temp retention %dh | max upload %dMB",
		c.StorageBackend, c.StorageDir, c.TempRetentionHours, c.MaxUploadMB)
	log.Printf("  cors          %s", strings.Join(c.CORSOrigins, ", "))
}

func (c *Config) grpcStatus() string {
	if c.CoreTTSGrpcURL != "" {
		return "gRPC + HTTP (gRPC for Synthesize, HTTP for Manifest/Voices/Clone)"
	}
	return "HTTP only"
}

// str reads a string variable according to the specification.
func str(key string) string {
	s := lookup(key)
	if v, exists := os.LookupEnv(s.Key); exists && strings.TrimSpace(v) != "" {
		return v
	}
	return s.Default
}

// num reads an integer variable according to the specification.
//
// An unparseable value will stop the process, rather than silently falling back to the default.
// A mistyped variable — "twenty", "32MB" — was previously discarded without a trace, so the
// operator believed the configuration was in effect while the system ran with a different
// value. Failing immediately at startup is far easier to fix than a silent failure.
func num(key string) int {
	s := lookup(key)

	raw, exists := os.LookupEnv(s.Key)
	if !exists || strings.TrimSpace(raw) == "" {
		raw = s.Default
	}

	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		log.Fatalf("Environment variable %s = %q is not a valid integer. Fix it or unset the variable to use the default (%s).", s.Key, raw, s.Default)
	}
	if val < s.Min || val > s.Max {
		log.Fatalf("Environment variable %s = %d is outside the allowed range (%d to %d).", s.Key, val, s.Min, s.Max)
	}
	return val
}
