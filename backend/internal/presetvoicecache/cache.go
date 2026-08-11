package presetvoicecache

import (
	"sync"
	"time"

	"backend/client"
)

// DefaultTTL là tuổi tối đa của một bản nạp giọng preset trước khi bị coi là cũ.
// Danh sách giọng của engine đổi hiếm, nên 30 giây tránh các lượt gọi lặp khi người dùng mở
// voice picker nhưng vẫn bắt kịp thay đổi ngay cả khi engine quên tăng manifest version.
const DefaultTTL = 30 * time.Second

type entry struct {
	voices          []client.CoreVoice
	manifestVersion string
	fetched         time.Time
}

// Cache giữ danh sách giọng preset dùng chung theo mode. Giọng preset không thuộc về cá nhân;
// giọng clone của người dùng không đi qua component này.
type Cache struct {
	mu              sync.Mutex
	mode            map[string]*entry
	ttl             time.Duration
	manifestVersion func() string
}

// New tạo cache với TTL và nguồn cung cấp manifest version hiện tại. Callback giữ package này
// độc lập với global state của handler và cho phép mỗi deployment quyết định nơi giữ manifest.
func New(ttl time.Duration, manifestVersion func() string) *Cache {
	if manifestVersion == nil {
		manifestVersion = func() string { return "" }
	}
	return &Cache{
		mode:            make(map[string]*entry),
		ttl:             ttl,
		manifestVersion: manifestVersion,
	}
}

// Get trả bản cache nếu cả TTL lẫn manifest version còn hợp lệ. Entry hết hạn bị loại ngay để
// map không tích tụ dữ liệu không còn dùng.
func (c *Cache) Get(mode string, now time.Time) ([]client.CoreVoice, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(mode)
	e := c.mode[key]
	if e == nil {
		return nil, false
	}
	if e.manifestVersion != c.manifestVersion() || now.Sub(e.fetched) > c.ttl {
		delete(c.mode, key)
		return nil, false
	}
	return e.voices, true
}

// Store lưu một bản nạp giọng preset kèm version tại thời điểm lấy từ engine.
func (c *Cache) Store(mode string, voices []client.CoreVoice, now time.Time) {
	c.mu.Lock()
	c.mode[cacheKey(mode)] = &entry{
		voices:          voices,
		manifestVersion: c.manifestVersion(),
		fetched:         now,
	}
	c.mu.Unlock()
}

func cacheKey(mode string) string {
	if mode == "" {
		return "all"
	}
	return mode
}
