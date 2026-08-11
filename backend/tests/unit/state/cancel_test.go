package state_test

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
