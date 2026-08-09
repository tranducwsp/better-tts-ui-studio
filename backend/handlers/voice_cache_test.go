package handlers

import (
	"testing"
	"time"

	"backend/client"
	"backend/state"
	"backend/types"
)

// voiceModeManifest dựng một manifest hợp lệ dùng được ngay (Set() từ chối manifest rỗng).
func voiceModeManifest(version string) *types.UniversalManifest {
	return &types.UniversalManifest{
		EngineID:       "cache-test-engine",
		EngineName:     "Cache Test",
		Version:        version,
		Provider:       "test",
		SupportedModes: []types.EngineModeSpec{{ID: "standard", Name: "Standard"}},
	}
}

// clearPresetVoiceCache dọn mọi entry cache để không rò sang bài test khác.
func clearPresetVoiceCache() {
	presetVoiceCache.mu.Lock()
	defer presetVoiceCache.mu.Unlock()
	presetVoiceCache.mode = make(map[string]*voiceCacheEntry)
}

// TestPresetVoiceCache_Hit cùng khoá chuẩn hoá: mode rỗng ("toàn cảnh") được lưu dưới khoá
// "all" (voiceCacheKey), và đọc lại bằng "all" phải trúng đúng bản đó.
func TestPresetVoiceCache_Hit(t *testing.T) {
	if err := state.GlobalManifestState.Set(voiceModeManifest("v1")); err != nil {
		t.Fatalf("manifest hợp lệ mà bị từ chối: %v", err)
	}
	t.Cleanup(state.GlobalManifestState.Clear)
	t.Cleanup(clearPresetVoiceCache)

	now := time.Now()
	storeCachedPresetVoices("", []client.CoreVoice{{ID: "v-1", Name: "Voice one"}}, now)

	got, ok := getCachedPresetVoices("all", now.Add(time.Second))
	if !ok {
		t.Fatal("entry vừa lưu phải đọc lại được")
	}
	if len(got) != 1 || got[0].ID != "v-1" {
		t.Fatalf("cache trả về sai nội dung: %+v", got)
	}
}

// TestPresetVoiceCache_ManifestChangeInvalidates xác nhận engine reload (admin kích, version
// manifest vừa đổi) khiến bản cache lấy theo manifest cũ hết giá trị ngay, không chờ TTL.
func TestPresetVoiceCache_ManifestChangeInvalidates(t *testing.T) {
	if err := state.GlobalManifestState.Set(voiceModeManifest("v1")); err != nil {
		t.Fatalf("manifest hợp lệ mà bị từ chối: %v", err)
	}
	t.Cleanup(state.GlobalManifestState.Clear)
	t.Cleanup(clearPresetVoiceCache)

	now := time.Now()
	storeCachedPresetVoices("standard", []client.CoreVoice{{Name: "old voice"}}, now)

	if err := state.GlobalManifestState.Set(voiceModeManifest("v2")); err != nil {
		t.Fatalf("manifest hợp lệ mà bị từ chối: %v", err)
	}

	if _, ok := getCachedPresetVoices("standard", now.Add(time.Second)); ok {
		t.Fatal("manifest version đổi thì cache cũ phải miss")
	}
}

// TestPresetVoiceCache_ExpiresByTTL: không có gì đổi version, chỉ quá TTL — cache vẫn phải
// hết hạn để bắt kịp khi engine đổi giọng mà không bump version.
func TestPresetVoiceCache_ExpiresByTTL(t *testing.T) {
	if err := state.GlobalManifestState.Set(voiceModeManifest("v1")); err != nil {
		t.Fatalf("manifest hợp lệ mà bị từ chối: %v", err)
	}
	t.Cleanup(state.GlobalManifestState.Clear)
	t.Cleanup(clearPresetVoiceCache)

	now := time.Now()
	storeCachedPresetVoices("standard", []client.CoreVoice{{Name: "old voice"}}, now)

	if _, ok := getCachedPresetVoices("standard", now.Add(voiceCacheTTL+time.Second)); ok {
		t.Fatal("cache quá TTL phải miss")
	}
}