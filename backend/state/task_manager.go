package state

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

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

	// Đọc trạng thái đã ghi trên Redis trước khi dựng bản mới, thay vì ghi đè sạch trơn.
	//
	// Tiến trình web ghi trạng thái task lên Redis lúc nhận yêu cầu (Synthesize) — và có thể đã
	// ghi luôn cờ huỷ vào đó nếu người dùng bấm dừng ngay sau khi gửi. Worker chạy ở tiến trình
	// riêng không thấy RAM của web, nên bản này là nguồn duy nhất của nó. Ghi đè bằng một
	// TaskItem "sạch" (Cancel=false, Status=processing) chính là cách một lệnh huỷ biến mất:
	// worker nhặt job bị huỷ, giẫm lên cancel:true vừa lưu, rồi chạy trọn lượt mà người dùng đã
	// bấm dừng — DB và giao diện sau đó báo "done" đè lên "cancelled" đã hiện.
	if RedisClient != nil {
		ctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
		val, rerr := RedisClient.Get(ctx, "task:"+taskID).Result()
		rcancel()
		if rerr == nil && val != "" {
			var remote TaskItem
			if err := sonic.Unmarshal([]byte(val), &remote); err == nil {
				item.applyRemoteState(&remote)
			}
		}
	}

	tm.tasks[taskID] = item

	// Đăng ký Key lên Redis nếu có kết nối.
	//
	// Qua marshalState chứ không marshal cả struct: đó là đường ghi duy nhất, nên bản đầu và
	// mọi bản cập nhật về sau mang cùng một tập trường. Marshal trực tiếp *TaskItem còn đọc
	// các trường không giữ khoá, và ghi ra một payload khác hình dạng với mọi lượt ghi sau.
	if RedisClient != nil {
		ctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
		defer rcancel()
		if taskStateData, err := item.marshalState(); err == nil {
			_ = RedisClient.Set(ctx, "task:"+taskID, taskStateData, redisTaskTTL).Err()
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

// SetOwner gắn chủ sở hữu cho task nếu nó chưa có, và đẩy lên Redis nếu vừa gắn.
//
// Không ghi đè: task_id do client gửi lên, nên nếu một người gửi trùng task_id của người
// khác thì lượt sau không được phép chiếm quyền sở hữu lượt trước.
//
// Phải tự ghi Redis: GetOrCreate đã ghi bản đầu xong trước khi người gọi kịp biết chủ là ai,
// nên nếu chỉ sửa trong RAM thì replica khác đọc lại task sẽ thấy một task không chủ. Chỉ ghi
// khi thực sự có thay đổi — synth.Run cũng gọi hàm này trên task đã có chủ, và lượt đó không
// cần một round-trip nào.
func (t *TaskItem) SetOwner(userID string) {
	if userID == "" {
		return
	}
	t.mu.Lock()
	changed := t.OwnerID == ""
	if changed {
		t.OwnerID = userID
	}
	t.mu.Unlock()

	if !changed || RedisClient == nil {
		return
	}

	taskStateData, err := t.marshalState()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = RedisClient.Set(ctx, "task:"+t.ID, taskStateData, redisTaskTTL).Err()
}

// applyRemoteState cập nhật phần trạng thái điều khiển từ bản đọc được trên Redis.
//
// Chỉ chép những trường mà tiến trình khác có thẩm quyền hơn: tiến độ, kết quả, định dạng.
// Không đụng subscribers — người đăng ký SSE thuộc về tiến trình này. OwnerID được chép vì
// task dựng lại từ Redis mang theo chủ sở hữu đã ghi, và SetOwner chỉ gắn khi bản này chưa ai
// sở hữu. Cancel bắt buộc phải theo: nó là lệnh dừng do tiến trình web ghi, và worker khôi phục
// TaskItem từ Redis cần nhìn thấy nó để biết lượt tổng hợp phải cắt.
func (t *TaskItem) applyRemoteState(remote *TaskItem) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Status = remote.Status
	t.Progress = remote.Progress
	t.Error = remote.Error
	t.Cancel = remote.Cancel
	if remote.SourceFormat != "" {
		t.SourceFormat = remote.SourceFormat
	}
	if remote.OwnerID != "" && t.OwnerID == "" {
		t.OwnerID = remote.OwnerID
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
		status, sourceFormat, _, audio := item.Snapshot()

		// Bản trong RAM có thể đã cũ: tiến trình web tạo task lúc nhận request rồi đẩy sang
		// hàng đợi, nên bản của nó đứng yên ở "processing" trong khi worker ở tiến trình khác
		// đã chạy xong. Không đọc lại thì trạng thái không bao giờ tiến, dù công việc đã hoàn
		// tất và âm thanh đã nằm trong kho.
		//
		// Chỉ đọc lại khi chưa tới trạng thái cuối: task đã done/error/cancelled thì không đổi
		// nữa, và hỏi Redis mỗi lần chỉ thêm một lượt đi lại cho một câu trả lời đã biết.
		if RedisClient != nil && status != "done" && status != "error" && status != "cancelled" {
			rctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
			val, rerr := RedisClient.Get(rctx, "task:"+taskID).Result()
			rcancel()
			if err := rerr; err == nil && val != "" {
				var fresh TaskItem
				if err := sonic.Unmarshal([]byte(val), &fresh); err == nil {
					item.applyRemoteState(&fresh)
					status, sourceFormat, _, audio = item.Snapshot()
				}
			}
		}

		// Audio có thể do replica khác sinh ra, nên nạp bù từ Redis khi RAM rỗng. Ghi qua
		// SetAudio: trường này bị Notify đọc đồng thời, gán trực tiếp là data race.
		if RedisClient != nil && len(audio) == 0 && sourceFormat != "" && status == "done" {
			rctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
			b, err := RedisClient.Get(rctx, "task:audio:"+taskID+":"+sourceFormat).Bytes()
			rcancel()
			if err == nil {
				item.SetAudio(b)
			}
		}
		return item, true
	}

	// 2. Nếu RAM rỗng và có kết nối Redis (Hit sang Node khác), nạp Task từ Redis
	if RedisClient != nil {
		ctx, rcancel := context.WithTimeout(context.Background(), taskRedisTimeout)
		defer rcancel()
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

				item := tm.tasks[taskID]
				return item, true
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

// WatchCancel trả về một context bị huỷ khi task này được yêu cầu dừng.
//
// Phải đi qua Redis chứ không chỉ đọc cờ trong RAM: người bấm dừng nói chuyện với tiến trình
// web, còn lượt tổng hợp chạy ở worker — hai tiến trình khác nhau, không nhìn thấy RAM của
// nhau. Cancel phát "cancelled" lên channel:task:<id>, nên worker nghe chính kênh đó.
//
// Kiểm cờ cục bộ trước khi nghe: ở chế độ không Redis, cũng như khi lệnh dừng tới trước lúc
// worker kịp nhặt job, trạng thái đã nằm sẵn trong RAM và không có bản tin nào để chờ nữa.
//
// Người gọi phải gọi stop để giải phóng subscription và goroutine.

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

// Cleanup thu hồi các task đã kết thúc và không còn ai theo dõi.
//
// Chỉ xoá task ở trạng thái cuối. Trước đây xoá theo tuổi bất kể trạng thái, mà
// TTS_CLIENT_TIMEOUT_SECONDS cho phép tới một giờ: một job dài hơn ngưỡng này bị xoá giữa
// chừng, rồi lượt Get kế tiếp dựng một TaskItem THỨ HAI từ Redis. Tiến trình tổng hợp vẫn giữ
// con trỏ bản cũ, nên nó ghi âm thanh và trạng thái vào một đối tượng mà không handler nào
// còn đọc — khi kho ghi hỏng và bản RAM là phương án dự phòng duy nhất, âm thanh có thật
// nhưng người dùng nhận "Audio is not ready".
//
// Còn subscriber thì giữ lại: xoá khỏi map trong khi một luồng SSE đang mở khiến người xem
// tiếp theo đăng ký lên một đối tượng khác, và hai bản cùng tồn tại cho một task.
func (tm *TaskManager) Cleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	for id, t := range tm.tasks {
		if taskRetention > 0 && now.Sub(t.CreatedAt) <= taskRetention {
			continue
		}
		if !t.finishedAndIdle() {
			// Task chưa kết thúc vẫn phải có trần: worker chết giữa chừng thì không ai đặt nó
			// về trạng thái cuối, và giữ mãi là một chỗ rỉ bộ nhớ không bao giờ tự đóng. Trần
			// này rộng hơn ngưỡng thường để không cắt một job dài đang chạy thật.
			if now.Sub(t.CreatedAt) <= taskMaxLifetime {
				continue
			}
			log.Printf("Task %s vẫn ở %q sau %s — thu hồi khỏi RAM", id, t.currentStatus(), taskMaxLifetime)
		}
		delete(tm.tasks, id)
	}

	// Nếu map đã được dọn sạch hoàn toàn, tái khởi tạo map để giải phóng các buckets cũ của Go map cho GC thu hồi.
	if len(tm.tasks) == 0 {
		tm.tasks = make(map[string]*TaskItem)
	}
}

// finishedAndIdle cho biết task đã tới trạng thái cuối và không còn ai theo dõi.
func (t *TaskItem) finishedAndIdle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch t.Status {
	case "done", "error", "cancelled":
	default:
		return false
	}
	return len(t.subscribers) == 0
}

// currentStatus đọc trạng thái để ghi log.
func (t *TaskItem) currentStatus() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Status
}

func (tm *TaskManager) startCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		tm.Cleanup()
	}
}
