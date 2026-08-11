package presetvoicecache_test

import (
	"testing"
	"time"

	"backend/client"
	"backend/internal/presetvoicecache"
)

// TestCacheHitWithNormalizedMode xác nhận mode rỗng ("toàn cảnh") và khoá "all" cùng trỏ
// tới một entry, đúng với cách handler yêu cầu toàn bộ giọng preset.
func TestCacheHitWithNormalizedMode(t *testing.T) {
	version := "v1"
	cache := presetvoicecache.New(presetvoicecache.DefaultTTL, func() string { return version })
	now := time.Now()
	cache.Store("", []client.CoreVoice{{ID: "v-1", Name: "Voice one"}}, now)

	got, ok := cache.Get("all", now.Add(time.Second))
	if !ok {
		t.Fatal("entry vừa lưu phải đọc lại được")
	}
	if len(got) != 1 || got[0].ID != "v-1" {
		t.Fatalf("cache trả về sai nội dung: %+v", got)
	}
}

// TestManifestChangeInvalidates xác nhận engine reload và đổi manifest version làm bản cache cũ
// hết giá trị ngay, không cần chờ TTL.
func TestManifestChangeInvalidates(t *testing.T) {
	version := "v1"
	cache := presetvoicecache.New(presetvoicecache.DefaultTTL, func() string { return version })
	now := time.Now()
	cache.Store("standard", []client.CoreVoice{{Name: "old voice"}}, now)

	version = "v2"
	if _, ok := cache.Get("standard", now.Add(time.Second)); ok {
		t.Fatal("manifest version đổi thì cache cũ phải miss")
	}
}

// TestEntryExpiresByTTL xác nhận cache vẫn hết hạn khi engine đổi giọng nhưng không tăng version.
func TestEntryExpiresByTTL(t *testing.T) {
	cache := presetvoicecache.New(presetvoicecache.DefaultTTL, func() string { return "v1" })
	now := time.Now()
	cache.Store("standard", []client.CoreVoice{{Name: "old voice"}}, now)

	if _, ok := cache.Get("standard", now.Add(presetvoicecache.DefaultTTL+time.Second)); ok {
		t.Fatal("cache quá TTL phải miss")
	}
}
