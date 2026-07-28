package state

import (
	"context"
	"log"
	"sync"
	"time"

	"core-backend/config"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

var (
	// RedisClient kết nối tới Redis Server
	RedisClient *redis.Client
)

// InitRedis khởi tạo kết nối Redis Client từ cấu hình ENV.
func InitRedis(cfg *config.Config) {
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

type TaskUpdate struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Error    string `json:"error,omitempty"`
}

type TaskItem struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Progress    int       `json:"progress"`
	AudioWAV    []byte    `json:"audio_wav,omitempty"`
	AudioMP3    []byte    `json:"audio_mp3,omitempty"`
	Cancel      bool      `json:"cancel"`
	CreatedAt   time.Time `json:"created_at"`
	Error       string    `json:"error,omitempty"`
	subscribers map[chan TaskUpdate]struct{}
	mu          sync.Mutex
}

func (t *TaskItem) Subscribe() chan TaskUpdate {
	ch := make(chan TaskUpdate, 10)

	t.mu.Lock()
	if t.subscribers == nil {
		t.subscribers = make(map[chan TaskUpdate]struct{})
	}
	t.subscribers[ch] = struct{}{}
	t.mu.Unlock()

	// Nếu Redis đang bật, tạo Redis Pub/Sub Subscription để hỗ trợ Stateless Multi-Replica Streaming
	if RedisClient != nil {
		go func() {
			pubsub := RedisClient.Subscribe(context.Background(), "channel:task:"+t.ID)
			defer pubsub.Close()

			redisCh := pubsub.Channel()
			for msg := range redisCh {
				var update TaskUpdate
				if err := sonic.Unmarshal([]byte(msg.Payload), &update); err == nil {
					func() {
						defer func() { _ = recover() }()
						select {
						case ch <- update:
						default:
						}
					}()
				}
			}
		}()
	}

	return ch
}

func (t *TaskItem) Unsubscribe(ch chan TaskUpdate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.subscribers != nil {
		delete(t.subscribers, ch)
	}
}

func (t *TaskItem) Notify(update TaskUpdate) {
	t.mu.Lock()
	t.Status = update.Status
	t.Progress = update.Progress
	if update.Error != "" {
		t.Error = update.Error
	}
	subscribers := make([]chan TaskUpdate, 0, len(t.subscribers))
	for ch := range t.subscribers {
		subscribers = append(subscribers, ch)
	}
	t.mu.Unlock()

	// 1. Phát bản tin cho các subscriber cục bộ (Local In-Memory Subscriber)
	for _, ch := range subscribers {
		func(c chan TaskUpdate) {
			defer func() { _ = recover() }()
			select {
			case c <- update:
			default:
			}
		}(ch)
	}

	// 2. Nếu Redis hoạt động, Publish bản tin Pub/Sub và lưu State vào Redis cho các Node khác đọc
	if RedisClient != nil {
		ctx := context.Background()
		data, err := sonic.Marshal(update)
		if err == nil {
			// Redis PubSub Broadcast sang tất cả Replicas Backend khác
			_ = RedisClient.Publish(ctx, "channel:task:"+t.ID, string(data)).Err()
		}

		// Lưu thông tin Task vào Redis Key-Value (TTL 1 giờ)
		taskStateData, err := sonic.Marshal(t)
		if err == nil {
			_ = RedisClient.Set(ctx, "task:"+t.ID, string(taskStateData), 1*time.Hour).Err()
		}

		// Lưu Audio Bytes vào Redis Key nếu hoàn thành
		if len(t.AudioWAV) > 0 {
			_ = RedisClient.Set(ctx, "task:audio:"+t.ID+":wav", t.AudioWAV, 1*time.Hour).Err()
		}
		if len(t.AudioMP3) > 0 {
			_ = RedisClient.Set(ctx, "task:audio:"+t.ID+":mp3", t.AudioMP3, 1*time.Hour).Err()
		}
	}
}

type TaskManager struct {
	tasks map[string]*TaskItem
	mu    sync.RWMutex
}

var GlobalTaskManager = NewTaskManager()

func NewTaskManager() *TaskManager {
	tm := &TaskManager{
		tasks: make(map[string]*TaskItem),
	}
	go tm.startCleanupRoutine()
	return tm
}

func (tm *TaskManager) GetOrCreate(taskID string) *TaskItem {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if item, ok := tm.tasks[taskID]; ok {
		return item
	}

	item := &TaskItem{
		ID:          taskID,
		Status:      "processing",
		Progress:    0,
		CreatedAt:   time.Now(),
		subscribers: make(map[chan TaskUpdate]struct{}),
	}
	tm.tasks[taskID] = item

	// Đăng ký Key lên Redis nếu có kết nối
	if RedisClient != nil {
		ctx := context.Background()
		taskStateData, err := sonic.Marshal(item)
		if err == nil {
			_ = RedisClient.Set(ctx, "task:"+taskID, string(taskStateData), 1*time.Hour).Err()
		}
	}

	return item
}

func (tm *TaskManager) Get(taskID string) (*TaskItem, bool) {
	// 1. Kiểm tra RAM cục bộ trước
	tm.mu.RLock()
	item, ok := tm.tasks[taskID]
	tm.mu.RUnlock()

	if ok {
		// Kiểm tra thêm Audio từ Redis nếu RAM rỗng (trường hợp do Node khác tạo audio)
		if RedisClient != nil {
			if len(item.AudioWAV) == 0 {
				if wavBytes, err := RedisClient.Get(context.Background(), "task:audio:"+taskID+":wav").Bytes(); err == nil {
					item.AudioWAV = wavBytes
				}
			}
			if len(item.AudioMP3) == 0 {
				if mp3Bytes, err := RedisClient.Get(context.Background(), "task:audio:"+taskID+":mp3").Bytes(); err == nil {
					item.AudioMP3 = mp3Bytes
				}
			}
		}
		return item, true
	}

	// 2. Nếu RAM rỗng và có kết nối Redis (Hit sang Node khác), nạp Task từ Redis
	if RedisClient != nil {
		ctx := context.Background()
		val, err := RedisClient.Get(ctx, "task:"+taskID).Result()
		if err == nil && val != "" {
			var fetchedItem TaskItem
			if err := sonic.Unmarshal([]byte(val), &fetchedItem); err == nil {
				fetchedItem.subscribers = make(map[chan TaskUpdate]struct{})

				// Nạp Audio bytes từ Redis
				if wavBytes, err := RedisClient.Get(ctx, "task:audio:"+taskID+":wav").Bytes(); err == nil {
					fetchedItem.AudioWAV = wavBytes
				}
				if mp3Bytes, err := RedisClient.Get(ctx, "task:audio:"+taskID+":mp3").Bytes(); err == nil {
					fetchedItem.AudioMP3 = mp3Bytes
				}

				tm.mu.Lock()
				tm.tasks[taskID] = &fetchedItem
				tm.mu.Unlock()

				return &fetchedItem, true
			}
		}
	}

	return nil, false
}

func (tm *TaskManager) Cancel(taskID string) bool {
	item, ok := tm.Get(taskID)
	if !ok {
		return false
	}

	item.Cancel = true
	item.Notify(TaskUpdate{
		Status:   "cancelled",
		Progress: item.Progress,
	})
	return true
}

func (tm *TaskManager) Cleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	for id, t := range tm.tasks {
		if now.Sub(t.CreatedAt) > 10*time.Minute {
			delete(tm.tasks, id)
		}
	}
}

func (tm *TaskManager) startCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		tm.Cleanup()
	}
}
