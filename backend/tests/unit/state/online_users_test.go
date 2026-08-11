package state_test

import (
	"context"
	"testing"

	"backend/state"
)

// TestOnlineUsers_NoRedis trả về map rỗng thay vì panic khi chạy không có Redis.
func TestOnlineUsers_NoRedis(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	online := state.OnlineUsers(context.Background(), []string{"a", "b"})
	if len(online) != 0 {
		t.Errorf("không có Redis thì không ai online, nhận %v", online)
	}
}

// TestOnlineUsers_EmptyInput không gọi Redis khi danh sách rỗng: MGET không nhận zero key và
// sẽ trả lỗi, biến một trang không có người dùng nào thành một lỗi.
func TestOnlineUsers_EmptyInput(t *testing.T) {
	online := state.OnlineUsers(context.Background(), nil)
	if len(online) != 0 {
		t.Errorf("danh sách rỗng phải trả map rỗng, nhận %v", online)
	}
}
