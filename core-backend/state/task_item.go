package state

import (
	"context"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

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
	// OwnerID có mặt ở đây vì Notify là đường ghi duy nhất chạy sau khi chủ sở hữu được gắn:
	// GetOrCreate ghi bản đầu trước khi handler kịp gọi SetOwner. Thiếu nó thì mỗi lượt ghi
	// tiến độ lại xoá chủ sở hữu khỏi Redis, và replica nào đọc lại task cũng phải hỏi DB —
	// đúng cái giá mà trường này tồn tại để không phải trả.
	return sonic.Marshal(struct {
		ID           string    `json:"id"`
		Status       string    `json:"status"`
		Progress     int       `json:"progress"`
		OwnerID      string    `json:"owner_id,omitempty"`
		SourceFormat string    `json:"source_format,omitempty"`
		Cancel       bool      `json:"cancel"`
		CreatedAt    time.Time `json:"created_at"`
		Error        string    `json:"error,omitempty"`
	}{
		ID:           t.ID,
		Status:       t.Status,
		Progress:     t.Progress,
		OwnerID:      t.OwnerID,
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

func (t *TaskItem) WatchCancel(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	if t.IsCancelled() {
		cancel()
		return ctx, cancel
	}

	if RedisClient == nil {
		// Không Redis nghĩa là một tiến trình duy nhất: RequestCancel đặt cờ trong cùng RAM
		// này, nên hỏi định kỳ là đủ và không cần thêm hạ tầng nào.
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if t.IsCancelled() {
						cancel()
						return
					}
				}
			}
		}()
		return ctx, cancel
	}

	pubsub := RedisClient.Subscribe(ctx, "channel:task:"+t.ID)
	go func() {
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, open := <-ch:
				if !open {
					return
				}
				var update TaskUpdate
				if err := sonic.Unmarshal([]byte(msg.Payload), &update); err != nil {
					continue
				}
				if update.Status == "cancelled" {
					t.RequestCancel()
					cancel()
					return
				}
			}
		}
	}()

	return ctx, func() {
		cancel()
	}
}
