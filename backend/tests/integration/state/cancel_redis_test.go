package state_test

import (
	"context"
	"testing"
	"time"

	"backend/state"
)

// TestWatchCancel_CrossProcess kiểm tra cancel qua Redis giữa hai tiến trình.
func TestWatchCancel_CrossProcess(t *testing.T) {
	redisOrSkip(t)

	const taskID = "watch-cancel-cross"
	t.Cleanup(func() { state.RedisClient.Del(context.Background(), "task:"+taskID) })

	// Tiến trình "worker": đang chạy job và nghe lệnh dừng.
	worker := state.NewTaskManager()
	workerItem := worker.GetOrCreate(taskID)
	workerItem.Notify(state.TaskUpdate{Status: "processing", Progress: 10})

	ctx, stop := workerItem.WatchCancel(context.Background())
	defer stop()

	// Subscribe của go-redis nối lười; chờ một nhịp để nó kịp đăng ký trước khi phát.
	time.Sleep(300 * time.Millisecond)

	select {
	case <-ctx.Done():
		t.Fatal("context huỷ trước khi có ai yêu cầu dừng")
	default:
	}

	// Tiến trình "web": nhận cú bấm dừng của người dùng.
	web := state.NewTaskManager()
	if !web.Cancel(taskID) {
		t.Fatal("tiến trình web không huỷ được task")
	}

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("lệnh dừng ở tiến trình web không tới được lượt tổng hợp ở worker — GPU vẫn chạy hết")
	}

	if !workerItem.IsCancelled() {
		t.Error("worker chưa ghi nhận task đã bị huỷ")
	}
}
