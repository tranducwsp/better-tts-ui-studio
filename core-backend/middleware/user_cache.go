package middleware

import (
	"sync"
	"time"

	"core-backend/db/sqlc"
)

// userCache ghi nhớ bản ghi người dùng trong thời gian rất ngắn sau khi token được xác thực.
//
// Không có nó, mỗi request đã đăng nhập là một câu SELECT tới PostgreSQL để suy ra lại thứ
// gần như đã nằm trong token đã ký. Middleware này chạy trên chain toàn cục, nên nó đánh cả
// /health, /ready và preflight CORS; endpoint hỏi tiến độ task còn bị trình duyệt gọi liên
// tục trong lúc job chạy. Ở vài trăm request mỗi giây, phần lớn pool kết nối (mặc định 25)
// bị tiêu cho việc tra cùng một dòng.
//
// Chỉ role và trạng thái duyệt là những trường cần tươi, và cả hai đều xoá cache ngay khi
// thay đổi qua Invalidate — nên độ trễ ở đây chỉ áp dụng cho thay đổi đến từ nơi khác
// (sửa tay trong DB, một replica khác), và giới hạn bằng TTL.
type userCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]userCacheEntry
}

type userCacheEntry struct {
	user      sqlc.User
	expiresAt time.Time
}

// globalUserCache được cấu hình một lần lúc khởi động qua ConfigureUserCache.
var globalUserCache = &userCache{entries: make(map[string]userCacheEntry)}

// ConfigureUserCache đặt thời gian sống của cache. ttl <= 0 tắt cache hoàn toàn.
func ConfigureUserCache(ttl time.Duration) {
	globalUserCache.mu.Lock()
	globalUserCache.ttl = ttl
	globalUserCache.entries = make(map[string]userCacheEntry)
	globalUserCache.mu.Unlock()
}

// InvalidateUser xoá một người dùng khỏi cache, để lần xác thực sau đọc lại từ DB.
//
// Gọi ở mọi nơi làm thay đổi role hoặc trạng thái duyệt: nếu không, một tài khoản vừa được
// duyệt vẫn bị từ chối cho tới khi TTL hết, và người dùng không hiểu vì sao.
func InvalidateUser(username string) {
	globalUserCache.mu.Lock()
	delete(globalUserCache.entries, username)
	globalUserCache.mu.Unlock()
}

// get trả về bản ghi còn hiệu lực, hoặc ok=false nếu chưa có/đã hết hạn/cache đang tắt.
func (c *userCache) get(username string, now time.Time) (sqlc.User, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.ttl <= 0 {
		return sqlc.User{}, false
	}
	entry, ok := c.entries[username]
	if !ok || now.After(entry.expiresAt) {
		return sqlc.User{}, false
	}
	return entry.user, true
}

// put ghi nhớ một bản ghi, và tranh thủ dọn các entry đã hết hạn.
//
// Dọn ngay trong lúc ghi thay vì chạy một goroutine hẹn giờ: số khoá bị chặn bởi số người
// dùng đang hoạt động trong một khoảng TTL vài giây, nên map không kịp lớn đến mức cần một
// bộ quét riêng.
func (c *userCache) put(username string, user sqlc.User, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ttl <= 0 {
		return
	}
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	c.entries[username] = userCacheEntry{user: user, expiresAt: now.Add(c.ttl)}
}

// Hai hàm dưới đây cho phép kiểm cache mà không cần dựng PostgreSQL — đường vào thật là
// lookupUser, nhưng nó đi qua db.Queries nên không kiểm được độc lập.

// PutUserForTest ghi một bản ghi vào cache. Chỉ dùng trong kiểm thử.
func PutUserForTest(username string, user sqlc.User) {
	globalUserCache.put(username, user, time.Now())
}

// GetUserForTest đọc một bản ghi từ cache. Chỉ dùng trong kiểm thử.
func GetUserForTest(username string) (sqlc.User, bool) {
	return globalUserCache.get(username, time.Now())
}
