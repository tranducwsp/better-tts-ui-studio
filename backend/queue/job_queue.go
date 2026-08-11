package queue

import (
	"context"
	"errors"
	"time"

	"backend/state"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

// streamKey là stream chứa mọi yêu cầu tổng hợp đang chờ.
const streamKey = "tts:jobs"

// consumerGroup gom mọi worker vào một nhóm, để Redis chia job chứ không phát trùng.
const consumerGroup = "tts-workers"

// maxStreamLen là trần số job giữ trong stream.
//
// Stream này KHÔNG bền: Redis chạy không có AOF/RDB, nên mọi thứ trong đây mất khi Redis khởi
// động lại. Đó là lựa chọn có chủ ý — một chunk chỉ tốn vài giây để tổng hợp lại, và giao
// diện đã tự thử lại ba lần cho mỗi chunk. Đánh đổi lấy việc không phải nuôi thêm một kho bền
// vững, và không phải giữ trạng thái job đồng bộ giữa hai nơi.
//
// Trần này chỉ để một đợt dồn bất thường không ăn hết RAM của Redis; MAXLEN xấp xỉ nên Redis
// cắt theo khối, rẻ hơn cắt chính xác.
const maxStreamLen = 10000

// ErrNoRedis nghĩa là hàng đợi không dùng được vì thiếu Redis.
//
// Người gọi phía web dùng nó để quay về chạy tại chỗ: một triển khai không có Redis vẫn tổng
// hợp được, chỉ là không tách được worker.
var ErrNoRedis = errors.New("queue: chưa cấu hình Redis")

// Job là một yêu cầu tổng hợp đang chờ được xử lý.
//
// Mang đủ những gì worker cần để chạy mà không phải hỏi lại ai: worker có thể nằm ở tiến
// trình khác, máy khác, và không thấy được RAM của tiến trình đã nhận request.
type Job struct {
	TaskID  string   `json:"task_id"`
	UserID  string   `json:"user_id"`
	Text    string   `json:"text"`
	Voice   string   `json:"voice"`
	Engine  string   `json:"engine"`
	Speed   float64  `json:"speed"`
	Pitch   *float64 `json:"pitch,omitempty"`
	Emotion *string  `json:"emotion,omitempty"`
}

// Enqueue đẩy một job vào hàng đợi.
func Enqueue(ctx context.Context, job Job) error {
	rdb := state.RedisClient
	if rdb == nil {
		return ErrNoRedis
	}

	payload, err := sonic.Marshal(job)
	if err != nil {
		return err
	}

	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: maxStreamLen,
		Approx: true,
		Values: map[string]any{"job": payload},
	}).Err()
}

// EnsureGroup tạo consumer group nếu chưa có.
//
// MKSTREAM để worker khởi động trước web vẫn tạo được nhóm trên một stream chưa tồn tại.
// BUSYGROUP nghĩa là nhóm đã có — thường gặp khi chạy nhiều worker, và không phải lỗi.
func EnsureGroup(ctx context.Context) error {
	rdb := state.RedisClient
	if rdb == nil {
		return ErrNoRedis
	}

	err := rdb.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// Consume đọc job cho tới khi ctx bị huỷ, gọi handle cho từng job.
//
// consumer đặt tên riêng từng tiến trình để Redis phân biệt được các worker trong cùng nhóm.
//
// Ack ngay sau khi handle trả về, kể cả khi nó báo lỗi: job hỏng đã được ghi trạng thái error
// và giao diện sẽ tự thử lại, nên giữ nó trong danh sách pending chỉ tạo ra rác mà không ai
// đọc. Cùng lý do đó, ở đây không có XAUTOCLAIM: worker chết giữa chừng thì job mất luôn, và
// đó là hành vi đã chọn.
func Consume(ctx context.Context, consumer string, handle func(context.Context, Job)) error {
	rdb := state.RedisClient
	if rdb == nil {
		return ErrNoRedis
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		// Block có thời hạn thay vì vô hạn: cần quay lại vòng lặp định kỳ để thấy ctx đã huỷ.
		res, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumer,
			Streams:  []string{streamKey, ">"},
			Count:    1,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) || ctx.Err() != nil {
				continue // hết thời gian chờ, không có job — bình thường
			}
			// Redis chớp tắt: chờ một nhịp rồi thử lại thay vì quay vòng nóng.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}

		for _, stream := range res {
			for _, msg := range stream.Messages {
				raw, _ := msg.Values["job"].(string)

				var job Job
				if err := sonic.Unmarshal([]byte(raw), &job); err != nil {
					// Không giải mã được thì không ai xử lý được; ack để nó không nằm lại mãi.
					_ = rdb.XAck(ctx, streamKey, consumerGroup, msg.ID).Err()
					continue
				}

				handle(ctx, job)
				_ = rdb.XAck(ctx, streamKey, consumerGroup, msg.ID).Err()
			}
		}
	}
}
