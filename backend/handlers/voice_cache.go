package handlers

import (
	"sync"
	"time"

	"backend/client"
	"backend/state"
)

// voiceCacheTTL là tuổi tối đa của một bản nạp giọng preset trước khi bị coi là cũ.
//
// Danh sách giọng của engine đổi hiếm, nên 30 giây dài hơn mọi lần mở/đóng voice picker mà
// vẫn giữ được phản hồi khi engine đổi giọng. Ngắn hơn nghĩa là nhiều lượt gọi HTTP vô ích
// về engine; dài hơn nghĩa là người vận hành thấy giọng mới mãi không kịp xuất hiện.
const voiceCacheTTL = 30 * time.Second

// voiceCacheEntry là một bản nạp giọng preset theo mode, kèm chứng nhận thời điểm và version.
type voiceCacheEntry struct {
	voices          []client.CoreVoice
	manifestVersion string
	fetched         time.Time
}

// presetVoiceCache là kho cache chung cho mọi người dùng — giọng preset không thuộc về cá
// nhân, nên một bản cache chia sẻ là an toàn (khác với giọng clone, vốn phải tách theo user).
var presetVoiceCache = struct {
	mu   sync.Mutex
	mode map[string]*voiceCacheEntry
}{mode: make(map[string]*voiceCacheEntry)}

// voiceCacheKey chuẩn hoá mode trống (hỏi "toàn cảnh") về "all" để không giữ hai bản dư thừa.
func voiceCacheKey(mode string) string {
	if mode == "" {
		return "all"
	}
	return mode
}

// getCachedPresetVoices lấy giọng preset đã cache nếu vẫn còn mới qua HAI lớp:
//
//   - manifestVersion: admin reload manifest (engine có thể lúc đó đổi giọng) khiến bản cache
//     lấy theo manifest cũ hết giá trị ngay — không phải chờ tới lúc TTL hết.
//   - voiceCacheTTL: phòng trường hợp manifest không bump version dù giọng đã đổi.
//
// Bản hết hạn bị xoá khỏi map luôn, không để tồn lại tích tụ. Trả về voices và true khi đọc
// được; false nghĩa là caller nên nạp lại.
func getCachedPresetVoices(mode string, now time.Time) ([]client.CoreVoice, bool) {
	presetVoiceCache.mu.Lock()
	defer presetVoiceCache.mu.Unlock()

	e := presetVoiceCache.mode[voiceCacheKey(mode)]
	if e == nil {
		return nil, false
	}
	if e.manifestVersion != currentManifestVersion() || now.Sub(e.fetched) > voiceCacheTTL {
		delete(presetVoiceCache.mode, voiceCacheKey(mode))
		return nil, false
	}
	return e.voices, true
}

// storeCachedPresetVoices lưu một bản nạp giọng preset thành cache.
func storeCachedPresetVoices(mode string, voices []client.CoreVoice, now time.Time) {
	presetVoiceCache.mu.Lock()
	presetVoiceCache.mode[voiceCacheKey(mode)] = &voiceCacheEntry{
		voices:           voices,
		manifestVersion:  currentManifestVersion(),
		fetched:          now,
	}
	presetVoiceCache.mu.Unlock()
}

// currentManifestVersion lấy version của manifest đang dùng; rỗng khi chưa có.
func currentManifestVersion() string {
	if m := state.GlobalManifestState.Get(); m != nil {
		return m.Version
	}
	return ""
}