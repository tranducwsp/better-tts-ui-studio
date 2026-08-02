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

	{Key: "DATABASE_URL", Kind: KindString, Default: "postgres://postgres:postgres@localhost:5432/ai_studio?sslmode=disable", Group: "Database"},
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
	{Key: "REDIS_PASSWORD", Kind: KindSecret, Default: "", Group: "Redis (optional)"},

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

	{Key: "STORAGE_DIR", Kind: KindString, Default: "storage", Group: "Storage"},
	{
		Key: "MAX_UPLOAD_SIZE_MB", Kind: KindInt, Default: "32", Min: 1, Max: 10240,
		Group: "Storage",
		Doc:   "Xem PLATFORM_GAPS.md §2: giới hạn này lẽ ra thuộc về manifest của engine.",
	},
	{
		Key: "TEMP_AUDIO_RETENTION_HOURS", Kind: KindInt, Default: "24", Min: 1, Max: 8760,
		Group: "Storage",
		Doc: "Số giờ giữ âm thanh đã sinh trong storage/temp. Bộ quét chạy mỗi giờ và một lần lúc\n" +
			"khởi động; không có nó thư mục sẽ phình lên suốt vòng đời triển khai.",
	},

	{
		Key: "CORS_ALLOWED_ORIGINS", Kind: KindList,
		Default: "http://localhost:5173,http://localhost:3000,http://127.0.0.1:5173",
		Group:   "CORS",
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
