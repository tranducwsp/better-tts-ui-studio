package state

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

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
	// Ghi trạng thái TRƯỚC khi Publish, cố ý: WatchCancel của worker đọc lại khoá task:<id>
	// ngay sau khi subscribe, và Cancel dựa trên thứ tự này để đóng khe hở tin Publish bị mất
	// trước lúc worker kịp đăng ký. Nếu bản ghi có trước lượt đọc thì worker thấy cờ huỷ; nếu
	// lượt đọc chạy trước bản ghi thì Publish tới sau, tới đúng subscription vừa sẵn sàng. Đảo
	// ngược thứ tự, một lần chạy có thể lọt giữa "Set chưa xong" và "Publish đã đi qua lúc chưa
	// ai nghe" — đúng cái khe đang được đóng.
	pipe.Set(ctx, "task:"+t.ID, taskStateData, redisTaskTTL)
	pipe.Publish(ctx, "channel:task:"+t.ID, data)
	_, _ = pipe.Exec(ctx)
}

// CacheAudio lưu bytes âm thanh lên Redis dưới khoá riêng theo định dạng.
//
// Tách khỏi Notify có chủ ý: âm thanh xuất hiện đúng một lần khi chunk xong, còn Notify
// chạy trên mọi mốc tiến độ. Gộp chung khiến cùng một payload được ghi lại ở mỗi mốc.
