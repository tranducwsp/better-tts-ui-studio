package state_test

import (
	"context"
	"testing"

	"backend/state"
)

// TestOnlineUsers_NoRedis returns an empty map instead of panicking when running without Redis.
func TestOnlineUsers_NoRedis(t *testing.T) {
	prev := state.RedisClient
	state.RedisClient = nil
	t.Cleanup(func() { state.RedisClient = prev })

	online := state.OnlineUsers(context.Background(), []string{"a", "b"})
	if len(online) != 0 {
		t.Errorf("without Redis no one is online, got %v", online)
	}
}

// TestOnlineUsers_EmptyInput does not call Redis with an empty list: MGET does not accept
// zero keys and would error, turning a page with no users into an error.
func TestOnlineUsers_EmptyInput(t *testing.T) {
	online := state.OnlineUsers(context.Background(), nil)
	if len(online) != 0 {
		t.Errorf("empty list must return an empty map, got %v", online)
	}
}
