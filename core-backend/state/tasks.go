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

// redisTaskTTL là thời gian sống của trạng thái task và bytes âm thanh trên Redis.
const redisTaskTTL = 1 * time.Hour

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
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`

	// OwnerID là user đã tạo task này, dùng để kiểm quyền truy cập.
	//
	// Nằm ở đây thay vì phải join DB trên mỗi lượt hỏi: polling trạng thái và SSE là hai
	// đường đi nóng nhất của một job đang chạy, nên một truy vấn cho mỗi lần hỏi tiến độ là
	// cái giá không cần trả. Rỗng nghĩa là chưa biết — task được dựng lại từ Redis sau khi
	// khởi động lại không mang theo trường này, và người kiểm quyền phải hỏi DB.
	OwnerID string `json:"owner_id,omitempty"`

	// Audio là dữ liệu Engine trả về, ở đúng định dạng SourceFormat khai. Trước đây trường
	// này tên AudioWAV và mọi nơi coi nó là WAV — trong khi Mode chạy Edge TTS trả MP3, nên
	// tệp tải xuống mang phần mở rộng .wav mà bên trong là MP3.
	//
	// json:"-" vì trạng thái task được marshal lên Redis trên mỗi mốc tiến độ, còn bytes âm
	// thanh nằm dưới khoá task:audio:<id>:<format> riêng. Để chúng ở đây nghĩa là đẩy lại
	// cả đoạn âm thanh mỗi lần báo một con số phần trăm.
	Audio        []byte `json:"-"`
	SourceFormat string `json:"source_format,omitempty"`

	// Bản đã chuyển mã, ghi nhớ theo định dạng để không chạy lại ffmpeg mỗi lần tải.
	Transcoded map[string][]byte `json:"-"`

	Cancel      bool      `json:"cancel"`
	CreatedAt   time.Time `json:"created_at"`
	Error       string    `json:"error,omitempty"`
	subscribers map[chan TaskUpdate]struct{}
	mu          sync.Mutex

	// pubsubCancel đóng subscription Redis dùng chung của task này. Nil nghĩa là chưa có
	// ai xem, hoặc người xem cuối đã rời đi.
	pubsubCancel     func()
	pubsubGeneration uint64
}

func (t *TaskItem) Subscribe() chan TaskUpdate {
	ch := make(chan TaskUpdate, 10)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.subscribers == nil {
		t.subscribers = make(map[chan TaskUpdate]struct{})
	}
	t.subscribers[ch] = struct{}{}

	// Một subscription Redis cho mỗi task, không phải cho mỗi người xem: hai tab mở cùng
	// một task đăng ký cùng một channel, nên nhân bản subscription chỉ khiến Redis gửi
	// đúng payload đó hai lần rồi ta tự phân phát lại trong tiến trình.
	if RedisClient != nil && t.pubsubCancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		pubsub := RedisClient.Subscribe(ctx, "channel:task:"+t.ID)
		t.pubsubGeneration++
		generation := t.pubsubGeneration
		t.pubsubCancel = func() {
			cancel()
			_ = pubsub.Close()
		}
		go t.forwardRedis(ctx, pubsub, generation)
	}

	return ch
}

// forwardRedis chuyển bản tin từ Redis về cho các subscriber cục bộ của task này.
//
// Vòng lặp kết thúc khi pubsub bị đóng — chính là điều Unsubscribe làm khi người xem cuối
// cùng rời đi. Trước đây hàm này chạy với context.Background() và không ai đóng pubsub, nên
// mỗi lần mở SSE là một goroutine cùng một subscription Redis sống mãi: trình duyệt tự kết
// nối lại sau mỗi lần đứt mạng, và số subscription trên Redis chỉ có tăng.
func (t *TaskItem) forwardRedis(ctx context.Context, pubsub *redis.PubSub, generation uint64) {
	redisCh := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, open := <-redisCh:
			if !open {
				return
			}
			var update TaskUpdate
			if err := sonic.Unmarshal([]byte(msg.Payload), &update); err != nil {
				continue
			}
			t.fanoutIfCurrent(update, generation)
		}
	}
}

// fanoutIfCurrent lấy snapshot subscriber dưới cùng một khoá với việc kiểm tra generation.
// Nếu chỉ kiểm tra generation rồi mới gọi fanout, goroutine cũ có thể nhìn thấy mình còn
// hợp lệ, bị unsubscribe, rồi fanout đúng lúc subscription mới đã được tạo — subscriber
// mới sẽ nhận cùng một bản tin hai lần.
func (t *TaskItem) fanoutIfCurrent(update TaskUpdate, generation uint64) {
	t.mu.Lock()
	if t.pubsubGeneration != generation || t.pubsubCancel == nil {
		t.mu.Unlock()
		return
	}
	for ch := range t.subscribers {
		select {
		case ch <- update:
		default:
		}
	}
	t.mu.Unlock()
}

// fanout gửi bản tin cho mọi subscriber cục bộ, bỏ qua ai đang tắc.
//
// Kênh có đệm 10 và nhánh default bỏ tin: một trình duyệt đọc chậm không được phép làm
// nghẽn tiến trình tổng hợp đứng sau.
func (t *TaskItem) fanout(update TaskUpdate) {
	t.mu.Lock()
	subscribers := make([]chan TaskUpdate, 0, len(t.subscribers))
	for ch := range t.subscribers {
		subscribers = append(subscribers, ch)
	}
	t.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- update:
		default:
		}
	}
}

func (t *TaskItem) Unsubscribe(ch chan TaskUpdate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.subscribers != nil {
		delete(t.subscribers, ch)
	}

	// Người xem cuối cùng rời đi thì đóng subscription, nếu không goroutine chuyển tin sẽ
	// còn lại một mình cùng một entry trên Redis mà không ai đọc.
	if len(t.subscribers) == 0 && t.pubsubCancel != nil {
		cancel := t.pubsubCancel
		t.pubsubCancel = nil
		t.pubsubGeneration++ // invalidates any final message already buffered by the old forwarder
		cancel()
	}
}

func (t *TaskItem) Notify(update TaskUpdate) {
	t.mu.Lock()
	t.Status = update.Status
	t.Progress = update.Progress
	if update.Error != "" {
		t.Error = update.Error
	}
	t.mu.Unlock()

	// Khi Redis tắt, phát trực tiếp trong tiến trình. Khi Redis bật, để subscription dùng
	// chung phát bản tin kể cả cho subscriber cùng node — nếu phát cả hai đường thì chính
	// bản Publish của node này quay lại qua Redis rồi fanout lần hai, SSE nhận trùng event.
	if RedisClient == nil {
		t.fanout(update)
	}

	// Nếu Redis hoạt động, Publish cho các Node khác và lưu trạng thái để chúng đọc.
	//
	// Một pipeline thay vì bốn round-trip tuần tự: hàm này chạy trên mỗi mốc tiến độ, nên
	// bốn lần chờ mạng ở đây nằm thẳng trong đường đi của tiến trình tổng hợp.
	//
	// Audio KHÔNG còn đi kèm ở đây. Trước đây trạng thái task được marshal nguyên khối kể
	// cả trường Audio, nên mỗi mốc tiến độ đẩy lại toàn bộ đoạn âm thanh — một chunk 3 MB
	// nhân với mười mốc là 30 MB lưu lượng lặp lại để nói một con số phần trăm. Bytes âm
	// thanh có khoá riêng, ghi một lần khi đã có (xem CacheAudio).
	if RedisClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := sonic.Marshal(update)
	if err != nil {
		return
	}
	taskStateData, err := t.marshalState()
	if err != nil {
		return
	}

	pipe := RedisClient.Pipeline()
	pipe.Publish(ctx, "channel:task:"+t.ID, data)
	pipe.Set(ctx, "task:"+t.ID, taskStateData, redisTaskTTL)
	_, _ = pipe.Exec(ctx)
}

// CacheAudio lưu bytes âm thanh lên Redis dưới khoá riêng theo định dạng.
//
// Tách khỏi Notify có chủ ý: âm thanh xuất hiện đúng một lần khi chunk xong, còn Notify
// chạy trên mọi mốc tiến độ. Gộp chung khiến cùng một payload được ghi lại ở mỗi mốc.
func (t *TaskItem) CacheAudio(ctx context.Context, format string, data []byte) {
	if RedisClient == nil || format == "" || len(data) == 0 {
		return
	}
	_ = RedisClient.Set(ctx, "task:audio:"+t.ID+":"+format, data, redisTaskTTL).Err()
}

// Ba hàm dưới đây tồn tại vì TaskItem được đọc bởi request goroutine trong khi tiến trình
// tổng hợp đang ghi: chạm thẳng vào trường là data race mà -race sẽ báo.

// SetSourceFormat ghi lại định dạng Engine vừa trả về.
func (t *TaskItem) SetSourceFormat(format string) {
	t.mu.Lock()
	t.SourceFormat = format
	t.mu.Unlock()
}

// SetAudio giữ bytes âm thanh trong RAM, dùng khi không ghi được ra đĩa.
func (t *TaskItem) SetAudio(data []byte) {
	t.mu.Lock()
	t.Audio = data
	t.mu.Unlock()
}

// ReleaseAudio thả bản trong RAM sau khi âm thanh đã nằm an toàn trên đĩa.
func (t *TaskItem) ReleaseAudio() {
	t.mu.Lock()
	t.Audio = nil
	t.Transcoded = nil
	t.mu.Unlock()
}

func (t *TaskItem) marshalState() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Serialize only the small control-plane state. Audio and transcodes live in their own
	// Redis keys and on disk; including them here would make every progress update grow with
	// the audio payload and would also require copying it under the lock.
	return sonic.Marshal(struct {
		ID           string    `json:"id"`
		Status       string    `json:"status"`
		Progress     int       `json:"progress"`
		SourceFormat string    `json:"source_format,omitempty"`
		Cancel       bool      `json:"cancel"`
		CreatedAt    time.Time `json:"created_at"`
		Error        string    `json:"error,omitempty"`
	}{
		ID:           t.ID,
		Status:       t.Status,
		Progress:     t.Progress,
		SourceFormat: t.SourceFormat,
		Cancel:       t.Cancel,
		CreatedAt:    t.CreatedAt,
		Error:        t.Error,
	})
}

// Snapshot đọc những trường mà handler cần, dưới một lần giữ khoá.
func (t *TaskItem) Snapshot() (status, sourceFormat string, progress int, audio []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Status, t.SourceFormat, t.Progress, t.Audio
}

// LastError trả về thông báo lỗi gần nhất, rỗng nếu task chưa hỏng.
func (t *TaskItem) LastError() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Error
}

// RequestCancel đánh dấu task cần dừng. Tiến trình tổng hợp đọc cờ này giữa các bước.
func (t *TaskItem) RequestCancel() {
	t.mu.Lock()
	t.Cancel = true
	t.mu.Unlock()
}

// IsCancelled cho biết task đã bị yêu cầu dừng hay chưa.
func (t *TaskItem) IsCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Cancel
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

// Owner trả về user sở hữu task, và false nếu task chưa ghi nhận chủ.
func (t *TaskItem) Owner() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.OwnerID, t.OwnerID != ""
}

// SetOwner gắn chủ sở hữu cho task nếu nó chưa có.
//
// Không ghi đè: task_id do client gửi lên, nên nếu một người gửi trùng task_id của người
// khác thì lượt sau không được phép chiếm quyền sở hữu lượt trước.
func (t *TaskItem) SetOwner(userID string) {
	if userID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.OwnerID == "" {
		t.OwnerID = userID
	}
}

// Peek trả về task trong RAM tiến trình này, không nạp bù gì từ Redis.
//
// Get nạp âm thanh từ Redis vào RAM khi thấy RAM rỗng — đúng cho người gọi sắp phục vụ bytes,
// nhưng sai cho người chỉ muốn biết định dạng: nó kéo cả tệp vào RAM để trả lời một câu hỏi
// vài ký tự, và làm hỏng nhánh chuyển hướng thẳng tới kho.
func (tm *TaskManager) Peek(taskID string) (*TaskItem, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	item, ok := tm.tasks[taskID]
	return item, ok
}

func (tm *TaskManager) Get(taskID string) (*TaskItem, bool) {
	// 1. Kiểm tra RAM cục bộ trước
	tm.mu.RLock()
	item, ok := tm.tasks[taskID]
	tm.mu.RUnlock()

	if ok {
		// Audio có thể do replica khác sinh ra, nên nạp bù từ Redis khi RAM rỗng. Ghi qua
		// SetAudio: trường này bị Notify đọc đồng thời, gán trực tiếp là data race.
		status, sourceFormat, _, audio := item.Snapshot()
		if RedisClient != nil && len(audio) == 0 && sourceFormat != "" && status == "done" {
			if b, err := RedisClient.Get(context.Background(), "task:audio:"+taskID+":"+sourceFormat).Bytes(); err == nil {
				item.SetAudio(b)
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

				// Nạp Audio bytes từ Redis theo định dạng gốc đã ghi trong Task.
				if fetchedItem.SourceFormat != "" {
					if b, err := RedisClient.Get(ctx, "task:audio:"+taskID+":"+fetchedItem.SourceFormat).Bytes(); err == nil {
						fetchedItem.Audio = b
					}
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

// CacheTranscoded ghi nhớ một bản đã chuyển mã để lần tải sau không phải chạy lại ffmpeg.
//
// Chỉ ghi ra đĩa và Redis, không giữ trong RAM: bản chuyển mã to xấp xỉ bản gốc, và giữ
// mỗi định dạng người dùng từng tải là nhân dung lượng thường trú lên theo số định dạng.
// Đĩa đã có bộ quét dọn định kỳ, Redis đã có TTL — cả hai đều có giới hạn, RAM thì không.
func (tm *TaskManager) CacheTranscoded(taskID, format string, data []byte) {
	if len(data) == 0 || format == "" {
		return
	}

	tm.mu.RLock()
	_, ok := tm.tasks[taskID]
	tm.mu.RUnlock()
	if !ok {
		return
	}

	if RedisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = RedisClient.Set(ctx, "task:audio:"+taskID+":"+format, data, redisTaskTTL).Err()
	}
}

func (tm *TaskManager) Cancel(taskID string) bool {
	item, ok := tm.Get(taskID)
	if !ok {
		return false
	}

	item.RequestCancel()
	_, _, progress, _ := item.Snapshot()
	item.Notify(TaskUpdate{
		Status:   "cancelled",
		Progress: progress,
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
