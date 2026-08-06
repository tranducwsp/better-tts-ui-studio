package tests

import (
	"testing"
	"time"

	"core-backend/state"
)

// TestCleanup_KeepsRunningTask là bất biến mà đường tổng hợp dài dựa vào.
//
// Trước đây Cleanup xoá theo tuổi bất kể trạng thái, mà TTS_CLIENT_TIMEOUT_SECONDS cho phép
// tới một giờ. Một job dài hơn ngưỡng bị xoá giữa chừng, rồi lượt Get kế tiếp dựng một
// TaskItem THỨ HAI từ Redis — trong khi synth.Run vẫn giữ con trỏ bản cũ. Khi kho ghi hỏng và
// bản RAM là phương án dự phòng duy nhất, âm thanh nằm ở bản A còn handler đọc bản B.
func TestCleanup_KeepsRunningTask(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	tm := state.NewTaskManager()
	item := tm.GetOrCreate("cleanup-running")
	item.Notify(state.TaskUpdate{Status: "processing", Progress: 40})
	item.CreatedAt = time.Now().Add(-30 * time.Minute)

	tm.Cleanup()

	if _, ok := tm.Peek("cleanup-running"); !ok {
		t.Error("task đang chạy bị thu hồi khỏi RAM — tiến trình tổng hợp sẽ ghi vào một bản không ai đọc")
	}
}

// TestCleanup_KeepsWatchedTask giữ task còn người xem SSE.
//
// Xoá khỏi map trong khi một luồng SSE đang mở khiến người xem tiếp theo đăng ký lên một đối
// tượng khác, và hai bản cùng tồn tại cho một task.
func TestCleanup_KeepsWatchedTask(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	tm := state.NewTaskManager()
	item := tm.GetOrCreate("cleanup-watched")
	item.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	item.CreatedAt = time.Now().Add(-30 * time.Minute)

	ch := item.Subscribe()
	defer item.Unsubscribe(ch)

	tm.Cleanup()

	if _, ok := tm.Peek("cleanup-watched"); !ok {
		t.Error("task còn người xem SSE bị thu hồi")
	}
}

// TestCleanup_RemovesFinishedTask giữ chiều ngược lại: đã xong và không ai xem thì phải thu
// hồi, nếu không map chỉ có lớn lên.
func TestCleanup_RemovesFinishedTask(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	tm := state.NewTaskManager()
	item := tm.GetOrCreate("cleanup-finished")
	item.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	item.CreatedAt = time.Now().Add(-30 * time.Minute)

	tm.Cleanup()

	if _, ok := tm.Peek("cleanup-finished"); ok {
		t.Error("task đã xong và không ai xem lẽ ra phải được thu hồi")
	}
}

// TestCleanup_RemovesAbandonedTask là trần tuyệt đối.
//
// Worker chết giữa chừng thì không ai đặt task về trạng thái cuối. Không có trần thì những
// task đó ở lại vĩnh viễn — đúng chỗ rỉ bộ nhớ mà việc giữ task đang chạy có thể tạo ra.
func TestCleanup_RemovesAbandonedTask(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	tm := state.NewTaskManager()
	item := tm.GetOrCreate("cleanup-abandoned")
	item.Notify(state.TaskUpdate{Status: "processing", Progress: 40})
	item.CreatedAt = time.Now().Add(-3 * time.Hour)

	tm.Cleanup()

	if _, ok := tm.Peek("cleanup-abandoned"); ok {
		t.Error("task mắc kẹt quá trần tuổi lẽ ra phải được thu hồi")
	}
}
