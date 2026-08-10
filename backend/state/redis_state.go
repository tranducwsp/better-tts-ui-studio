package state

import (
	"context"
	"log"
	"strings"
	"time"

	"backend/config"

	"github.com/redis/go-redis/v9"
)

var (
	// RedisClient kết nối tới Redis Server
	RedisClient *redis.Client
)

// redisTaskTTL là thời gian sống của trạng thái task và bytes âm thanh trên Redis.
const redisTaskTTL = 1 * time.Hour

// taskRetention là tuổi tối thiểu trước khi một task đã kết thúc được thu hồi khỏi RAM.
// Có thể tùy chỉnh bằng biến ENV TASK_MEMORY_RETENTION_SECONDS (đặt 0 để tắt In-Memory Cache).
var taskRetention = 10 * time.Minute

// ConfigureTaskRetention cập nhật thời gian giữ task trong RAM từ cấu hình ENV.
func ConfigureTaskRetention(d time.Duration) {
	taskRetention = d
	if d == 0 {
		log.Println("⚡ In-Memory Task Cache bị TẮT (TASK_MEMORY_RETENTION_SECONDS=0): Task hoàn thành sẽ được giải phóng khỏi RAM ngay sau khi xử lý.")
	} else {
		log.Printf("🧠 In-Memory Task Cache retention được cấu hình: %v", d)
	}
}

// taskMaxLifetime là trần tuyệt đối cho một task ở trong RAM, kể cả khi chưa kết thúc.
const taskMaxLifetime = 2 * time.Hour

// taskRedisTimeout chặn thời gian một lượt đọc/ghi trạng thái task trên Redis.
const taskRedisTimeout = 100 * time.Millisecond

// InitRedis khởi tạo kết nối Redis Client từ cấu hình ENV.
func InitRedis(cfg *config.Config) {
	ConfigureTaskRetention(time.Duration(cfg.TaskMemoryRetentionSeconds) * time.Second)

	if cfg.RedisURL == "" {
		log.Println("RedisURL không được cấu hình, TaskManager sử dụng In-Memory Mode.")
		return
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisURL,
		Password: cfg.RedisPassword,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Khong the ket noi den Redis server (%v). Fallback sang In-Memory Mode.", err)
		return
	}

	RedisClient = rdb
	log.Printf("Ket noi Redis thanh cong tai %s! Hệ thống đã chuyển sang Stateless Multi-Node Ready.", cfg.RedisURL)
}

// TouchUserOnline gia hạn trạng thái Online của người dùng trong Redis với TTL 60 giây.
func TouchUserOnline(ctx context.Context, userID string) {
	if RedisClient != nil && userID != "" {
		_ = RedisClient.Set(ctx, "user:online:"+userID, "1", 60*time.Second).Err()
	}
}

// IsUserOnline kiểm tra người dùng có đang Online hay không dựa trên Redis Key.
func IsUserOnline(ctx context.Context, userID string) bool {
	if RedisClient != nil && userID != "" {
		val, err := RedisClient.Exists(ctx, "user:online:"+userID).Result()
		return err == nil && val > 0
	}
	return false
}

// OnlineUsers cho biết những ai trong danh sách đang Online, trong MỘT lượt đi lại.
//
// Tách khỏi IsUserOnline vì trang quản trị hỏi cho cả bảng người dùng: gọi hàm kia trong vòng
// lặp là một round-trip cho mỗi hàng, và độ trễ của trang tỉ lệ thuận với số người dùng —
// đúng lúc Redis nằm ở máy khác thì thấy rõ nhất.
//
// MGET thay vì nhiều EXISTS: một lệnh, một lượt chờ mạng. Khoá không tồn tại trả nil, nên
// "có giá trị" chính là "đang online".
//
// Đóng khung thời gian như mọi lượt đọc Redis khác trên đường đi của request: trạng thái
// online là thông tin trang trí, không đáng để giữ một request lại khi Redis chậm.
func OnlineUsers(ctx context.Context, userIDs []string) map[string]bool {
	online := make(map[string]bool, len(userIDs))
	if RedisClient == nil || len(userIDs) == 0 {
		return online
	}

	keys := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id != "" {
			keys = append(keys, "user:online:"+id)
		}
	}
	if len(keys) == 0 {
		return online
	}

	opCtx, cancel := context.WithTimeout(ctx, taskRedisTimeout)
	defer cancel()

	vals, err := RedisClient.MGet(opCtx, keys...).Result()
	if err != nil {
		return online
	}

	for i, v := range vals {
		if v != nil {
			online[strings.TrimPrefix(keys[i], "user:online:")] = true
		}
	}
	return online
}
