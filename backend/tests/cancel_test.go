package tests

import (
	"context"
	"testing"
	"time"

	"backend/state"
)

// TestWatchCancel_FiresOnLocalFlag là nhánh không-Redis: một tiến trình duy nhất, cờ nằm
// trong cùng RAM.
func TestWatchCancel_FiresOnLocalFlag(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	item := state.GlobalTaskManager.GetOrCreate("watch-cancel-local")

	ctx, stop := item.WatchCancel(context.Background())
	defer stop()

	select {
	case <-ctx.Done():
		t.Fatal("context huỷ trước khi có ai yêu cầu dừng")
	default:
	}

	item.RequestCancel()

	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("RequestCancel không cắt được context — lượt gọi Engine sẽ chạy hết dù đã huỷ")
	}
}

// TestWatchCancel_AlreadyCancelled bắt trường hợp lệnh dừng tới trước khi worker nhặt job.
func TestWatchCancel_AlreadyCancelled(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	item := state.GlobalTaskManager.GetOrCreate("watch-cancel-early")
	item.RequestCancel()

	ctx, stop := item.WatchCancel(context.Background())
	defer stop()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("task đã bị huỷ từ trước mà context vẫn còn sống")
	}
}

// TestWatchCancel_CrossProcess là lý do WatchCancel phải đi qua Redis.
//
// Người bấm dừng nói chuyện với tiến trình web; lượt tổng hợp chạy ở worker. Hai tiến trình
// không nhìn thấy RAM của nhau, nên cờ cục bộ không bao giờ tới được nơi cần tới. Bài này
// dựng đúng hình đó: hai TaskManager riêng biệt, cùng một Redis.
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
