package config

import "math"

// Bảng đặc tả cấu hình: nơi DUY NHẤT mô tả mọi biến môi trường của backend.
//
// Trước đây mỗi giá trị mặc định tồn tại ba bản chép tay — trong lời gọi getEnv, trong
// .env.example, và trong docs/CONFIGURATION.md — mà không gì bắt buộc chúng khớp nhau. Bây
// giờ .env.example được sinh ra từ bảng này (`go generate ./config`) và một bài test đối
// chiếu tệp đã commit với bảng, nên một bản lệch sẽ làm CI đỏ chứ không âm thầm sai lệch.
//
// Thêm một biến mới: thêm một dòng vào đây, đọc nó trong LoadConfig, rồi chạy go generate.
//
//go:generate go run ../cmd/gen-env

// SettingKind phân loại cách một giá trị được phân tích và kiểm tra.
type SettingKind int

const (
	KindString SettingKind = iota
	KindInt
	KindSecret // giá trị không bao giờ được in ra log hay ghi vào .env.example
	KindList   // chuỗi phân tách bằng dấu phẩy
)

// Setting mô tả đầy đủ một biến môi trường: tên, mặc định, khoảng hợp lệ và lý do tồn tại.
type Setting struct {
	Key     string
	Kind    SettingKind
	Default string
	Min     int // chỉ dùng cho KindInt
	Max     int // chỉ dùng cho KindInt
	Group   string
	Doc     string

	// Required đánh dấu biến mà việc bỏ trống là rủi ro thật sự chứ không phải tiện lợi.
	// Không làm dừng tiến trình — LoadConfig tự xử lý — nhưng .env.example sẽ nêu bật.
	Required bool

	// ReadBy ghi tên dịch vụ đọc biến này, nếu không phải backend. Một vài biến sống trong
	// bảng vì chúng thuộc cùng một liên kết với biến kề bên — người vận hành cần thấy cả
	// hai chiều ở một chỗ — nhưng LoadConfig không đụng tới chúng.
	ReadBy string
}

const (
	noMin = math.MinInt
	noMax = math.MaxInt
)

// Settings liệt kê theo thứ tự xuất hiện trong .env.example.
var Settings = []Setting{
	{
		Key: "SECRET_KEY", Kind: KindSecret, Default: "", Required: true,
		Group: "Required in production",
		Doc: "Ký JWT phiên đăng nhập. Bỏ trống thì backend tự sinh khoá ngẫu nhiên lúc khởi động: có\n" +
			"cảnh báo trong log, mọi phiên mất hiệu lực sau mỗi lần khởi động lại, và nhiều replica\n" +
			"không dùng chung được phiên vì mỗi bản ký bằng một khoá khác nhau.\n" +
			"  openssl rand -hex 32",
	},

	{
		Key: "POSTGRES_PASSWORD", Kind: KindSecret, Default: "", Required: true,
		Group:  "Required in production",
		ReadBy: "docker-compose (postgres, và DATABASE_URL của backend)",
		Doc: "Mật khẩu Postgres. docker-compose.yml dựng DATABASE_URL từ biến này và sẽ từ chối\n" +
			"khởi động nếu nó rỗng. Trước đây user/mật khẩu đều viết cứng là \"postgres\" ngay\n" +
			"trong compose, đồng thời cổng 5432 mở ra host — tức bất kỳ ai tới được máy đều vào\n" +
			"được cơ sở dữ liệu bằng thông tin đăng nhập ai cũng biết.\n" +
			"  REDIS_PASSWORD nằm ở nhóm Redis bên dưới và giờ cũng là bắt buộc.\n" +
			"  openssl rand -hex 24",
	},

	{
		Key: "DEFAULT_ADMIN_USERNAME", Kind: KindString, Default: "",
		Group: "Seed accounts",
		Doc: "Tài khoản tạo sẵn ở lần khởi động đầu nếu chưa tồn tại. Để trống toàn bộ nhóm này thì\n" +
			"không tài khoản nào được tạo và người dùng đầu tiên tự đăng ký qua giao diện. Một\n" +
			"username thiếu password sẽ bị bỏ qua.",
	},
	{Key: "DEFAULT_ADMIN_PASSWORD", Kind: KindSecret, Default: "", Group: "Seed accounts"},
	{Key: "DEFAULT_USER_USERNAME", Kind: KindString, Default: "", Group: "Seed accounts"},
	{Key: "DEFAULT_USER_PASSWORD", Kind: KindSecret, Default: "", Group: "Seed accounts"},

	{Key: "HOST", Kind: KindString, Default: "0.0.0.0", Group: "Server"},
	{Key: "PORT", Kind: KindString, Default: "8000", Group: "Server"},
	{
		Key: "ACCESS_TOKEN_EXPIRE_MINUTES", Kind: KindInt, Default: "10080", Min: 1, Max: 525600,
		Group: "Server", Doc: "Thời hạn token đăng nhập. Mặc định 7 ngày.",
	},
	{
		Key: "AUTH_USER_CACHE_SECONDS", Kind: KindInt, Default: "15", Min: 0, Max: 3600,
		Group: "Server",
		Doc: "Số giây ghi nhớ bản ghi người dùng sau khi xác thực token, để mỗi request không\n" +
			"phải hỏi lại PostgreSQL cùng một câu. Chỉ role và trạng thái duyệt cần tươi, nên\n" +
			"độ trễ vài giây là chấp nhận được; duyệt tài khoản sẽ xoá cache ngay lập tức.\n" +
			"Đặt 0 để tắt cache và quay về truy vấn từng request.",
	},

	{Key: "DATABASE_URL", Kind: KindString, Default: "postgres://postgres:<change>@localhost:5432/ai_studio?sslmode=disable", Group: "Database", Doc: "Connection string PostgreSQL. Khi dùng docker-compose, giá trị này được dựng từ POSTGRES_PASSWORD;\n\t\tđể <change> nếu tự triển khai và thay bằng credential thật."},
	{Key: "DB_MAX_CONNS", Kind: KindInt, Default: "25", Min: 1, Max: 10000, Group: "Database"},
	{
		Key: "DB_MIN_CONNS", Kind: KindInt, Default: "5", Min: 0, Max: 10000,
		Group: "Database", Doc: "Không được lớn hơn DB_MAX_CONNS.",
	},
	{Key: "DB_MAX_CONN_LIFETIME_MINUTES", Kind: KindInt, Default: "30", Min: 1, Max: 10080, Group: "Database"},
	{Key: "DB_MAX_CONN_IDLE_MINUTES", Kind: KindInt, Default: "15", Min: 1, Max: 10080, Group: "Database"},
	{Key: "DB_CONNECT_MAX_RETRIES", Kind: KindInt, Default: "10", Min: 1, Max: 1000, Group: "Database"},
	{Key: "DB_CONNECT_RETRY_INTERVAL_SECONDS", Kind: KindInt, Default: "2", Min: 1, Max: 3600, Group: "Database"},

	{
		Key: "REDIS_URL", Kind: KindString, Default: "localhost:6379",
		Group: "Redis (optional)",
		Doc:   "Không có Redis thì backend chạy chế độ in-memory, đủ dùng cho một replica.",
	},
	{
		Key: "REDIS_PASSWORD", Kind: KindSecret, Default: "", Group: "Redis (optional)",
		Doc: "Cũng được truyền vào redis-server --requirepass trong docker-compose.yml, nên để\n" +
			"trống là chạy Redis không mật khẩu. Trước đây Redis vừa không mật khẩu vừa publish\n" +
			"6379 ra host; giờ cổng chỉ còn trong network nội bộ, nhưng hãy đặt giá trị cho môi\n" +
			"trường thật.\n" +
			"  openssl rand -hex 24",
	},

	{
		Key: "CORE_ENGINE_URL", Kind: KindString, Default: "http://localhost:8001",
		Group: "Core TTS engine",
		Doc:   "Nơi backend lấy manifest và gửi yêu cầu tổng hợp.",
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
		Doc: "Backend gọi URL này khi manifest đổi, để builder dựng lại bundle prerender.\n" +
			"Tín hiệu không mang dữ liệu — builder tự lấy manifest qua VITE_BACKEND_URL.",
	},
	{
		Key: "VITE_BACKEND_URL", Kind: KindString, Default: "http://core-backend:8000",
		Group:  "Frontend builder",
		ReadBy: "frontend",
		Doc: "Chiều ngược lại: nơi builder và dev server tìm backend để lấy manifest.\n" +
			"Backend không đọc biến này — nó nằm đây vì là nửa còn lại của cùng một liên kết,\n" +
			"và tách riêng ra một chỗ khác thì người vận hành phải nhớ hai nơi.\n" +
			"Đây là biến khởi động: không thể lấy từ manifest, vì cần nó mới lấy được manifest.",
	},

	{
		Key: "STORAGE_BACKEND", Kind: KindString, Default: "local", Group: "Storage",
		Doc: "Nơi cất âm thanh và giọng tham chiếu: \"local\" (đĩa của tiến trình) hoặc \"s3\".\n" +
			"Mặc định local để `docker compose up` chạy được ngay mà không cần tài khoản nào.\n" +
			"Một giá trị lạ làm backend dừng khởi động thay vì lặng lẽ quay về local — tệp đi\n" +
			"vào chỗ không ai đọc chỉ lộ ra khi có người cần lại chúng.\n" +
			"Chọn \"s3\" khi web và worker chạy trên nhiều node: với \"local\", node này không\n" +
			"đọc được tệp node kia vừa ghi.",
	},
	{Key: "STORAGE_DIR", Kind: KindString, Default: "storage", Group: "Storage",
		Doc: "Gốc lưu trữ khi STORAGE_BACKEND=local. Bị bỏ qua với các backend khác."},

	{
		Key: "S3_BUCKET", Kind: KindString, Default: "", Group: "Storage (S3)",
		Doc: "Bắt buộc khi STORAGE_BACKEND=s3. Bucket được kiểm ngay lúc khởi động, nên tên sai\n" +
			"hay thiếu quyền làm backend dừng lại thay vì hỏng ở lần tổng hợp đầu tiên.",
	},
	{
		Key: "S3_REGION", Kind: KindString, Default: "us-east-1", Group: "Storage (S3)",
		Doc: "Vùng của bucket. Kho tự dựng như MinIO thường bỏ qua giá trị này, nhưng SDK vẫn\n" +
			"đòi một giá trị nên cứ để mặc định.",
	},
	{
		Key: "S3_ENDPOINT", Kind: KindString, Default: "", Group: "Storage (S3)",
		Doc: "Để trống với AWS thật. Đặt khi dùng MinIO, Cloudflare R2 hay DigitalOcean Spaces,\n" +
			"ví dụ https://s3.example.com.",
	},
	{
		Key: "S3_ACCESS_KEY_ID", Kind: KindSecret, Default: "", Group: "Storage (S3)",
		Doc: "Bỏ trống CẢ HAI khoá để dùng IAM role (IRSA trên k8s, instance profile trên EC2) —\n" +
			"an toàn hơn khoá tĩnh vì không có gì để rò rỉ. Khai một trong hai là lỗi cấu hình\n" +
			"và bị từ chối lúc khởi động.",
	},
	{Key: "S3_SECRET_ACCESS_KEY", Kind: KindSecret, Default: "", Group: "Storage (S3)"},
	{
		Key: "S3_FORCE_PATH_STYLE", Kind: KindInt, Default: "0", Min: 0, Max: 1, Group: "Storage (S3)",
		Doc: "Đặt 1 cho MinIO và phần lớn kho tự dựng: chúng phục vụ theo đường dẫn\n" +
			"(endpoint/bucket/key) thay vì theo tên miền con, vì tên miền con cần wildcard DNS.",
	},
	{
		Key: "S3_PREFIX", Kind: KindString, Default: "", Group: "Storage (S3)",
		Doc: "Tiền tố chung cho mọi khoá, để nhiều môi trường dùng chung một bucket mà không\n" +
			"giẫm lên nhau. Ví dụ \"prod\" hoặc \"staging\".",
	},
	{
		Key: "MAX_UPLOAD_SIZE_MB", Kind: KindInt, Default: "256", Min: 1, Max: 10240,
		Group: "Storage",
		Doc: "Trần cứng của hạ tầng cho một request tải lên — nói về RAM và băng thông của\n" +
			"deployment, không phải về model. Engine khai trần riêng trong\n" +
			"audio_spec.max_upload_bytes; cái nào chặt hơn thì thắng.",
	},
	{
		Key: "TEMP_AUDIO_RETENTION_HOURS", Kind: KindInt, Default: "24", Min: 1, Max: 8760,
		Group: "Storage",
		Doc: "Số giờ giữ âm thanh đã sinh trong storage/temp. Bộ quét chạy mỗi giờ và một lần lúc\n" +
			"khởi động; không có nó thư mục sẽ phình lên suốt vòng đời triển khai.",
	},

	{
		Key: "COOKIE_SECURE", Kind: KindInt, Default: "1", Min: 0, Max: 1,
		Group: "CORS",
		Doc: "Đặt cờ Secure trên cookie access_token, tức chỉ gửi cookie qua HTTPS. Mặc định bật.\n" +
			"Trước đây cờ này bị viết cứng thành false, nên kể cả khi triển khai sau TLS thì token\n" +
			"phiên vẫn đi được qua HTTP thường. Chỉ đặt 0 khi phát triển cục bộ trên http://localhost.",
	},

	{
		Key: "WORKER_MAX_IN_FLIGHT", Kind: KindInt, Default: "2", Min: 1, Max: 128,
		Group: "Advanced deployment tuning",
		Doc: "Số lượt tổng hợp tối đa đồng thời trong MỖI worker. Mặc định an toàn cho một Engine GPU;\n" +
			"tổng tải là số worker nhân giá trị này. AI engineer thường không cần đổi biến này.",
	},
	{
		Key: "TRANSCODE_MAX_CONCURRENCY", Kind: KindInt, Default: "2", Min: 1, Max: 128,
		Group: "Advanced deployment tuning",
		Doc: "Số tiến trình ffmpeg tối đa đồng thời trong MỖI web replica. Chỉ đổi khi operator biết\n" +
			"quota CPU/RAM của deployment; không liên quan đến capability của Core TTS Engine.",
	},
	{
		Key: "TRANSCODE_TIMEOUT_SECONDS", Kind: KindInt, Default: "60", Min: 1, Max: 3600,
		Group: "Advanced deployment tuning",
		Doc: "Thời gian tối đa cho một lượt chuyển mã audio. Đây là policy của platform, không phải\n" +
			"thời gian Engine tổng hợp.",
	},
	{
		Key: "AUTH_RATE_LIMIT_REQUESTS", Kind: KindInt, Default: "10", Min: 1, Max: 1000,
		Group: "Advanced deployment tuning",
		Doc: "Số request login/register tối đa trong một cửa sổ. Mặc định bảo vệ bcrypt khỏi dò mật khẩu;\n" +
			"chỉ operator đổi khi hiểu traffic và deployment.",
	},
	{
		Key: "AUTH_RATE_LIMIT_WINDOW_SECONDS", Kind: KindInt, Default: "60", Min: 1, Max: 3600,
		Group: "Advanced deployment tuning",
		Doc:   "Độ dài cửa sổ rate limit login/register.",
	},

	{
		Key: "CORS_ALLOWED_ORIGINS", Kind: KindList,
		Default: "http://localhost:5173,http://localhost:3000,http://127.0.0.1:5173",
		Group:   "CORS",
	},

	{
		Key: "TRUSTED_PROXIES", Kind: KindList, Default: "",
		Group: "Rate limiting",
		Doc: "Các dải CIDR (hoặc IP) của proxy đứng trước backend, ví dụ \"10.0.0.0/8,172.16.0.0/12\".\n" +
			"Chỉ khi kết nối đến từ một trong các dải này thì X-Forwarded-For mới được tin để lấy IP\n" +
			"người gọi. Bỏ trống nghĩa là không tin header đó bao giờ — đúng cho triển khai không có\n" +
			"proxy phía trước.\n" +
			"Vì sao cần: header này do client đặt được. Không có danh sách tin cậy thì hạn mức bị\n" +
			"khoá theo một giá trị người gọi tự chọn, nên đổi header mỗi lần là hạn mức không còn\n" +
			"tác dụng — đo được 200/200 request lọt qua một hạn mức 10/phút.",
	},
}

// lookup trả về đặc tả của một biến. Panic khi không tìm thấy: LoadConfig chỉ hỏi những
// khoá viết cứng trong mã nguồn, nên thiếu khoá là lỗi lập trình chứ không phải lỗi cấu
// hình của người vận hành — và một bài test duyệt toàn bộ để bắt sớm.
func lookup(key string) Setting {
	for _, s := range Settings {
		if s.Key == key {
			return s
		}
	}
	panic("cấu hình: khoá chưa được khai trong Settings: " + key)
}
