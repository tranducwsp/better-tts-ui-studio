package presetvoicecache_test

import (
	"testing"
	"time"

	"backend/client"
	"backend/internal/presetvoicecache"
)

// TestCacheHitWithNormalizedMode confirms that an empty mode ("all") and the key "all" point to
// the same entry, matching how the handler requests all preset voices.
func TestCacheHitWithNormalizedMode(t *testing.T) {
	version := "v1"
	cache := presetvoicecache.New(presetvoicecache.DefaultTTL, func() string { return version })
	now := time.Now()
	cache.Store("", []client.CoreVoice{{ID: "v-1", Name: "Voice one"}}, now)

	got, ok := cache.Get("all", now.Add(time.Second))
	if !ok {
		t.Fatal("just stored entry must be retrievable")
	}
	if len(got) != 1 || got[0].ID != "v-1" {
		t.Fatalf("cache returned wrong content: %+v", got)
	}
}

// TestManifestChangeInvalidates confirms that engine reload and manifest version change make the old cache
// stale immediately, without waiting for TTL.
func TestManifestChangeInvalidates(t *testing.T) {
	version := "v1"
	cache := presetvoicecache.New(presetvoicecache.DefaultTTL, func() string { return version })
	now := time.Now()
	cache.Store("standard", []client.CoreVoice{{Name: "old voice"}}, now)

	version = "v2"
	if _, ok := cache.Get("standard", now.Add(time.Second)); ok {
		t.Fatal("manifest version changed so old cache must miss")
	}
}

// TestEntryExpiresByTTL confirms that cache still expires when the engine changes voices but does not bump the version.
func TestEntryExpiresByTTL(t *testing.T) {
	cache := presetvoicecache.New(presetvoicecache.DefaultTTL, func() string { return "v1" })
	now := time.Now()
	cache.Store("standard", []client.CoreVoice{{Name: "old voice"}}, now)

	if _, ok := cache.Get("standard", now.Add(presetvoicecache.DefaultTTL+time.Second)); ok {
		t.Fatal("cache past TTL must miss")
	}
}
