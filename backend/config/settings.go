package config

import "math"

// Configuration specification table: the SINGLE place that describes every backend environment
// variable.
//
// Previously each default value existed in three manual copies — in the getEnv call, in
// .env.example, and in docs/CONFIGURATION.md — with nothing enforcing they matched. Now
// .env.example is generated from this table (`go generate ./config`) and a test compares the
// committed file against the table, so a mismatch turns CI red instead of silently drifting.
//
// To add a new variable: add a row here, read it in LoadConfig, then run go generate.
//
//go:generate go run ../cmd/gen-env

// SettingKind classifies how a value is parsed and validated.
type SettingKind int

const (
	KindString SettingKind = iota
	KindInt
	KindSecret // value that must never be printed to logs or written to .env.example
	KindList   // comma-separated string
)

// Setting fully describes an environment variable: name, default, valid range, and reason for
// existing.
type Setting struct {
	Key     string
	Kind    SettingKind
	Default string
	Min     int // only used for KindInt
	Max     int // only used for KindInt
	Group   string
	Doc     string

	// Required marks a variable whose omission is a real risk, not a convenience.
	// Does not stop the process — LoadConfig handles that — but .env.example will highlight it.
	Required bool

	// ReadBy names the service that reads this variable, if not the backend. Some variables
	// live in the table because they belong to the same coupling as a neighboring variable —
	// the operator needs to see both sides in one place — but LoadConfig does not touch them.
	ReadBy string
}

const (
	noMin = math.MinInt
	noMax = math.MaxInt
)

// Settings are listed in the order they appear in .env.example.
var Settings = []Setting{
	{
		Key: "SECRET_KEY", Kind: KindSecret, Default: "", Required: true,
		Group: "Required in production",
		Doc: "Signs JWT login sessions. REQUIRED: leaving it empty causes the backend to refuse startup\n" +
			"instead of generating a random key (previously it auto-generated, but a different key on\n" +
			"each restart invalidated all sessions and made multiple replicas unable to share sessions).\n" +
			"  openssl rand -hex 32",
	},

	{
		Key: "POSTGRES_PASSWORD", Kind: KindSecret, Default: "", Required: true,
		Group:  "Required in production",
		ReadBy: "docker-compose (postgres, and backend's DATABASE_URL)",
		Doc: "Postgres password. docker-compose.yml constructs DATABASE_URL from this variable and will\n" +
			"refuse to start if it is empty. Previously both user and password were hardcoded as\n" +
			"\"postgres\" right in compose, while port 5432 was exposed to the host — meaning anyone\n" +
			"who could reach the machine could access the database with credentials everyone knew.\n" +
			"  REDIS_PASSWORD is in the Redis group below and is now also required.\n" +
			"  openssl rand -hex 24",
	},

	{
		Key: "DEFAULT_ADMIN_USERNAME", Kind: KindString, Default: "",
		Group: "Seed accounts",
		Doc: "Account created on first startup if it does not already exist. Leave this entire group\n" +
			"empty to create no accounts, and the first user registers through the UI. A username\n" +
			"without a password is skipped.",
	},
	{Key: "DEFAULT_ADMIN_PASSWORD", Kind: KindSecret, Default: "", Group: "Seed accounts"},
	{Key: "DEFAULT_USER_USERNAME", Kind: KindString, Default: "", Group: "Seed accounts"},
	{Key: "DEFAULT_USER_PASSWORD", Kind: KindSecret, Default: "", Group: "Seed accounts"},

	{Key: "HOST", Kind: KindString, Default: "0.0.0.0", Group: "Server"},
	{Key: "PORT", Kind: KindString, Default: "8000", Group: "Server"},
	{
		Key: "ACCESS_TOKEN_EXPIRE_MINUTES", Kind: KindInt, Default: "15", Min: 1, Max: 525600,
		Group: "Server", Doc: "Login access token lifetime. Default 15 minutes; refresh tokens are used to issue new access tokens.",
	},
	{
		Key: "REFRESH_TOKEN_EXPIRE_MINUTES", Kind: KindInt, Default: "43200", Min: 60, Max: 525600,
		Group: "Server", Doc: "Refresh token lifetime. Refresh tokens are only stored in HttpOnly cookies and are not accepted in the Authorization header.",
	},
	{
		Key: "AUTH_USER_CACHE_SECONDS", Kind: KindInt, Default: "15", Min: 0, Max: 3600,
		Group: "Server",
		Doc: "Number of seconds to cache the user record after token authentication, so each request\n" +
			"does not have to re-query PostgreSQL for the same row. Only role and approval status\n" +
			"need to be fresh, so a few seconds of delay is acceptable; account approval clears the\n" +
			"cache immediately. Set to 0 to disable caching and query on every request.",
	},
	{
		Key: "TASK_MEMORY_RETENTION_SECONDS", Kind: KindInt, Default: "600", Min: 0, Max: 86400,
		Group: "Server",
		Doc: "Number of seconds to hold completed task state in the TaskManager RAM cache. Default 600 (10 minutes).\n" +
			"Set to 0 to COMPLETELY DISABLE the In-Memory Task Cache; completed tasks will be evicted from RAM\n" +
			"immediately and subsequent status/download queries will read directly from Redis/Database/Storage.",
	},
	{
		Key: "ENABLE_REQUEST_LOGGING", Kind: KindString, Default: "true",
		Group: "Server",
		Doc: "Enable or disable the HTTP Server's Request Logger (chiMiddleware.Logger).\n" +
			"Set to false to suppress per-request HTTP logs to stdout, optimizing performance and RAM under high load.",
	},
	{
		Key: "ENABLE_PPROF", Kind: KindString, Default: "false",
		Group: "Server",
		Doc: "Enable or disable the Go Profiler endpoint (/debug/pprof). Default false for security and production optimization.\n" +
			"Set to true when you need to inspect Heap/Goroutine memory profiles at /debug/pprof.",
	},
	{
		Key: "ENABLE_SWAGGER", Kind: KindString, Default: "true",
		Group: "Server",
		Doc: "Enable or disable Swagger UI API documentation (/swagger). Default true.\n" +
			"Set to false to hide the Swagger documentation endpoint in production.",
	},

	{Key: "DATABASE_URL", Kind: KindString, Default: "postgres://postgres:<change>@localhost:5432/ai_studio?sslmode=disable", Group: "Database", Doc: "PostgreSQL connection string. When using docker-compose, this value is built from POSTGRES_PASSWORD;\n\t\tleave <change> if self-deploying and replace with real credentials."},
	{Key: "DB_MAX_CONNS", Kind: KindInt, Default: "25", Min: 1, Max: 10000, Group: "Database"},
	{
		Key: "DB_MIN_CONNS", Kind: KindInt, Default: "5", Min: 0, Max: 10000,
		Group: "Database", Doc: "Must not be greater than DB_MAX_CONNS.",
	},
	{Key: "DB_MAX_CONN_LIFETIME_MINUTES", Kind: KindInt, Default: "30", Min: 1, Max: 10080, Group: "Database"},
	{Key: "DB_MAX_CONN_IDLE_MINUTES", Kind: KindInt, Default: "15", Min: 1, Max: 10080, Group: "Database"},
	{Key: "DB_CONNECT_MAX_RETRIES", Kind: KindInt, Default: "10", Min: 1, Max: 1000, Group: "Database"},
	{Key: "DB_CONNECT_RETRY_INTERVAL_SECONDS", Kind: KindInt, Default: "2", Min: 1, Max: 3600, Group: "Database"},

	{
		Key: "REDIS_URL", Kind: KindString, Default: "localhost:6379",
		Group: "Redis (optional)",
		Doc:   "Without Redis the backend runs in in-memory mode, sufficient for a single replica.",
	},
	{
		Key: "REDIS_PASSWORD", Kind: KindSecret, Default: "", Group: "Redis (optional)",
		Doc: "Also passed to redis-server --requirepass in docker-compose.yml, so leaving it empty\n" +
			"runs Redis without a password. Previously Redis had no password and published port 6379\n" +
			"to the host; now the port is only on the internal network, but set a value for real\n" +
			"environments.\n" +
			"  openssl rand -hex 24",
	},

	{
		Key: "CORE_ENGINE_URL", Kind: KindString, Default: "http://localhost:8001",
		Group: "Core TTS engine",
		Doc:   "Where the backend fetches the manifest and sends synthesis requests.",
	},
	{
		Key: "CORE_ENGINE_GRPC_URL", Kind: KindString, Default: "localhost:50051",
		Group: "Core TTS engine",
	},
	{
		Key: "TTS_CLIENT_TIMEOUT_SECONDS", Kind: KindInt, Default: "60", Min: 1, Max: 3600,
		Group: "Core TTS engine",
	},
	{
		Key: "FE_BUILDER_URL", Kind: KindString, Default: "http://frontend-builder:3001",
		Group: "Frontend builder",
		Doc: "The backend calls this URL when the manifest changes, so the builder can rebuild the prerender\n" +
			"bundle. The signal carries no data — the builder fetches the manifest itself via VITE_BACKEND_URL.",
	},
	{
		Key: "VITE_BACKEND_URL", Kind: KindString, Default: "http://backend:8000",
		Group:  "Frontend builder",
		ReadBy: "frontend",
		Doc: "The reverse direction: where the builder and dev server find the backend to fetch the manifest.\n" +
			"The backend does not read this variable — it is here because it is the other half of the same\n" +
			"coupling, and splitting it elsewhere would force the operator to remember two places.\n" +
			"This is a startup variable: it cannot be fetched from the manifest, because it is needed to\n" +
			"fetch the manifest.",
	},

	{
		Key: "STORAGE_BACKEND", Kind: KindString, Default: "local", Group: "Storage",
		Doc: "Where to store audio and reference voices: \"local\" (process disk) or \"s3\".\n" +
			"Default local so `docker compose up` works immediately without any account.\n" +
			"An unknown value causes the backend to refuse startup rather than silently falling back\n" +
			"to local — files going to a place no one reads only surfaces when someone needs them back.\n" +
			"Choose \"s3\" when web and worker run on multiple nodes: with \"local\", one node cannot\n" +
			"read files another node just wrote.",
	},
	{Key: "STORAGE_DIR", Kind: KindString, Default: "storage", Group: "Storage",
		Doc: "Storage root when STORAGE_BACKEND=local. Ignored for other backends."},

	{
		Key: "S3_BUCKET", Kind: KindString, Default: "", Group: "Storage (S3)",
		Doc: "Required when STORAGE_BACKEND=s3. The bucket is verified at startup, so a wrong name or\n" +
			"missing permissions causes the backend to stop rather than failing on the first synthesis.",
	},
	{
		Key: "S3_REGION", Kind: KindString, Default: "us-east-1", Group: "Storage (S3)",
		Doc: "The bucket's region. Self-hosted stores like MinIO typically ignore this value, but the SDK\n" +
			"still requires one so leave the default.",
	},
	{
		Key: "S3_ENDPOINT", Kind: KindString, Default: "", Group: "Storage (S3)",
		Doc: "Leave empty for real AWS. Set when using MinIO, Cloudflare R2, or DigitalOcean Spaces,\n" +
			"e.g. https://s3.example.com.",
	},
	{
		Key: "S3_ACCESS_KEY_ID", Kind: KindSecret, Default: "", Group: "Storage (S3)",
		Doc: "Leave BOTH keys empty to use an IAM role (IRSA on k8s, instance profile on EC2) — safer\n" +
			"than static keys because there is nothing to leak. Providing one without the other is a\n" +
			"configuration error and is rejected at startup.",
	},
	{Key: "S3_SECRET_ACCESS_KEY", Kind: KindSecret, Default: "", Group: "Storage (S3)"},
	{
		Key: "S3_FORCE_PATH_STYLE", Kind: KindInt, Default: "0", Min: 0, Max: 1, Group: "Storage (S3)",
		Doc: "Set to 1 for MinIO and most self-hosted stores: they serve by path (endpoint/bucket/key)\n" +
			"instead of by subdomain, since subdomains require wildcard DNS.",
	},
	{
		Key: "S3_PREFIX", Kind: KindString, Default: "", Group: "Storage (S3)",
		Doc: "Common prefix for all keys, so multiple environments can share a single bucket without\n" +
			"stepping on each other. Example \"prod\" or \"staging\".",
	},
	{
		Key: "MAX_UPLOAD_SIZE_MB", Kind: KindInt, Default: "50", Min: 1, Max: 10240,
		Group: "Storage limits",
		Doc: "Infrastructure hard cap for a single upload request — about the deployment's RAM and\n" +
			"bandwidth, not about the model. The engine declares its own cap in\n" +
			"audio_spec.max_upload_bytes; whichever is tighter wins.",
	},
	{
		Key: "TEMP_AUDIO_RETENTION_HOURS", Kind: KindInt, Default: "24", Min: 1, Max: 8760,
		Group: "Storage limits",
		Doc: "Number of hours to keep generated audio in storage/temp. The sweeper runs every hour and\n" +
			"once at startup; without it the directory would grow unbounded over the deployment lifetime.",
	},
	{
		Key: "PRESERVE_FILES", Kind: KindInt, Default: "0", Min: 0, Max: 1,
		Group: "Storage limits",
		Doc: "Set to 1 to keep reference files in storage when deleting a cloned voice. Default 0: files\n" +
			"are deleted along with the DB record. Use 1 only when you need audit or separate backup —\n" +
			"orphaned files are not auto-deleted and must be cleaned up manually.",
	},

	{
		Key: "COOKIE_SECURE", Kind: KindInt, Default: "1", Min: 0, Max: 1,
		Group: "Cookie security",
		Doc: "Set the Secure flag on the access_token cookie, meaning the cookie is only sent over HTTPS.\n" +
			"Enabled by default. Previously this flag was hardcoded to false, so even when deployed behind\n" +
			"TLS, session tokens could still travel over plain HTTP. Only set to 0 for local development\n" +
			"on http://localhost.",
	},

	{
		Key: "WORKER_MAX_IN_FLIGHT", Kind: KindInt, Default: "2", Min: 1, Max: 128,
		Group: "Advanced deployment tuning",
		Doc: "Maximum concurrent synthesis jobs per worker. Default is safe for one GPU Engine; total load\n" +
			"is the number of workers multiplied by this value. AI engineers typically do not need to\n" +
			"change this variable.",
	},
	{
		Key: "TRANSCODE_MAX_CONCURRENCY", Kind: KindInt, Default: "2", Min: 1, Max: 128,
		Group: "Advanced deployment tuning",
		Doc: "Maximum concurrent ffmpeg processes per web replica. Only change when the operator knows\n" +
			"the deployment's CPU/RAM quota; unrelated to the Core TTS Engine's capability.",
	},
	{
		Key: "TRANSCODE_TIMEOUT_SECONDS", Kind: KindInt, Default: "60", Min: 1, Max: 3600,
		Group: "Advanced deployment tuning",
		Doc: "Maximum time allowed for one audio transcode operation. This is a platform policy, not the\n" +
			"Engine's synthesis time.",
	},
	{
		Key: "STALE_CHUNK_AFTER_MINUTES", Kind: KindInt, Default: "30", Min: 1, Max: 1440,
		Group: "Advanced deployment tuning",
		Doc: "A chunk still in pending/processing beyond this threshold is considered orphaned (lost queue,\n" +
			"worker died, Redis restart) and is moved to error by cron. The threshold must be greater than\n" +
			"the maximum time a valid chunk can run — i.e. TTS_CLIENT_TIMEOUT_SECONDS plus worst-case queue\n" +
			"wait — otherwise a long job or deep queue backlog gets wrongly killed.",
	},
	{
		Key: "AUTH_RATE_LIMIT_REQUESTS", Kind: KindInt, Default: "10", Min: 1, Max: 1000,
		Group: "Advanced deployment tuning",
		Doc: "Maximum login/register requests per window. Default protects bcrypt from password guessing;\n" +
			"only the operator changes this when they understand the traffic and deployment.",
	},
	{
		Key: "AUTH_RATE_LIMIT_WINDOW_SECONDS", Kind: KindInt, Default: "60", Min: 1, Max: 3600,
		Group: "Advanced deployment tuning",
		Doc:   "Login/register rate limit window duration.",
	},

	{
		Key: "CORS_ALLOWED_ORIGINS", Kind: KindList,
		Default: "http://localhost:5173,http://localhost:3000,http://127.0.0.1:5173",
		Group:   "CORS",
	},

	{
		Key: "TRUSTED_PROXIES", Kind: KindList, Default: "",
		Group: "Rate limiting",
		Doc: "CIDR ranges (or IPs) of proxies in front of the backend, e.g. \"10.0.0.0/8,172.16.0.0/12\".\n" +
			"Only when the connection comes from one of these ranges is X-Forwarded-For trusted to\n" +
			"determine the caller IP. Leave empty to never trust that header — correct for deployments\n" +
			"with no fronting proxy.\n" +
			"Why it matters: this header is settable by the client. Without a trusted list, rate limits\n" +
			"are keyed by a value the caller chooses, so changing the header each time renders the limit\n" +
			"ineffective — 200/200 requests get through a 10/min limit.",
	},
}

// lookup returns the specification for a variable. Panics on miss: LoadConfig only asks for
// keys hardcoded in the source, so a missing key is a programming error, not an operator's
// configuration error — and a test sweeps the entire table to catch it early.
func lookup(key string) Setting {
	for _, s := range Settings {
		if s.Key == key {
			return s
		}
	}
	panic("config: key not declared in Settings: " + key)
}
