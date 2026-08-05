package middleware

import (
	"context"
	"log"
	"sync"
	"time"

	"core-backend/db/sqlc"
	"core-backend/state"

	"github.com/bytedance/sonic"
)

// userCache ghi nhớ bản ghi người dùng trong thời gian rất ngắn sau khi token được xác thực.
//
// Không có nó, mỗi request đã đăng nhập là một câu SELECT tới PostgreSQL để suy ra lại thứ
// gần như đã nằm trong token đã ký. Middleware này chạy trên chain toàn cục, nên nó đánh cả
// /health, /ready và preflight CORS; endpoint hỏi tiến độ task còn bị trình duyệt gọi liên
// tục trong lúc job chạy. Ở vài trăm request mỗi giây, phần lớn pool kết nối (mặc định 25)
// bị tiêu cho việc tra cùng một dòng.
//
// Bộ nhớ nằm trên Redis khi có, RAM tiến trình khi không. Khác biệt không chỉ là chỗ chứa:
// InvalidateUser trước đây chỉ xoá bản sao của CHÍNH tiến trình đang chạy, nên với nhiều
// replica, admin duyệt một tài khoản ở replica A trong khi replica B vẫn từ chối người đó cho
// tới khi TTL hết — và người dùng bấm lại vài lần thì lúc được lúc không, tuỳ load balancer.
// Xoá trên Redis là xoá cho mọi replica cùng lúc.
type userCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]userCacheEntry
}

type userCacheEntry struct {
	user      sqlc.User
	expiresAt time.Time
}

// cachedUser là hình dạng được ghi lên Redis.
//
// Không mang PasswordHash: mọi thứ ngoài tiến trình đều là một bản sao nữa của thông tin
// nhạy cảm cần bảo vệ, và bcrypt hash nằm trong một Redis không mã hoá là chỗ rò rỉ không
// đổi lại lợi ích nào. Đã kiểm: chỉ Login đọc trường này, và nó đọc thẳng từ PostgreSQL chứ
// không qua context.
type cachedUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	IsApproved bool   `json:"is_approved"`
}

func (c cachedUser) toUser() sqlc.User {
	return sqlc.User{
		ID:         c.ID,
		Username:   c.Username,
		Role:       c.Role,
		IsApproved: c.IsApproved,
	}
}

func fromUser(u sqlc.User) cachedUser {
	return cachedUser{
		ID:         u.ID,
		Username:   u.Username,
		Role:       u.Role,
		IsApproved: u.IsApproved,
	}
}

// globalUserCache được cấu hình một lần lúc khởi động qua ConfigureUserCache.
var globalUserCache = &userCache{entries: make(map[string]userCacheEntry)}

// userCacheKey đặt tiền tố để khoá của cache này không đụng khoá nào khác trên cùng Redis.
func userCacheKey(username string) string { return "auth:user:" + username }

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
//
// Xoá cả hai chỗ, không chỉ chỗ đang dùng: một tiến trình có thể đã ghi vào RAM trước khi
// Redis kết nối được, và bỏ sót bản đó nghĩa là bản ghi cũ vẫn sống thêm một TTL nữa.
func InvalidateUser(username string) {
	globalUserCache.mu.Lock()
	delete(globalUserCache.entries, username)
	globalUserCache.mu.Unlock()

	if rdb := state.RedisClient; rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), cacheOpTimeout)
		defer cancel()
		if err := rdb.Del(ctx, userCacheKey(username)).Err(); err != nil {
			// Đáng ghi log: bản ghi cũ sẽ sống thêm tới hết TTL trên mọi replica, nên một tài
			// khoản vừa đổi quyền có thể vẫn hành xử theo quyền cũ trong vài giây.
			log.Printf("cache người dùng: không xoá được %q trên Redis: %v", username, err)
		}
	}
}

// cacheOpTimeout chặn thời gian một thao tác cache.
//
// Cache nằm trên đường đi của MỌI request đã xác thực, nên một Redis chậm không được phép
// trở thành độ trễ của cả hệ thống: quá hạn thì coi như trượt cache và đi thẳng xuống
// PostgreSQL, chậm hơn nhưng vẫn đúng.
const cacheOpTimeout = 100 * time.Millisecond

// get trả về bản ghi còn hiệu lực, hoặc ok=false nếu chưa có/đã hết hạn/cache đang tắt.
func (c *userCache) get(username string, now time.Time) (sqlc.User, bool) {
	c.mu.RLock()
	ttl := c.ttl
	c.mu.RUnlock()

	if ttl <= 0 {
		return sqlc.User{}, false
	}

	if rdb := state.RedisClient; rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), cacheOpTimeout)
		defer cancel()

		raw, err := rdb.Get(ctx, userCacheKey(username)).Bytes()
		if err != nil {
			// Bao gồm cả redis.Nil (chưa có khoá) — cả hai đều chỉ nghĩa là phải hỏi DB.
			return sqlc.User{}, false
		}
		var cu cachedUser
		if err := sonic.Unmarshal(raw, &cu); err != nil {
			return sqlc.User{}, false
		}
		return cu.toUser(), true
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[username]
	if !ok || now.After(entry.expiresAt) {
		return sqlc.User{}, false
	}
	return entry.user, true
}

// put ghi nhớ một bản ghi.
//
// Trên Redis, TTL của chính khoá lo việc hết hạn. Ở nhánh RAM thì phải tự dọn, và việc đó
// làm ngay trong lúc ghi thay vì bằng một goroutine hẹn giờ: số khoá bị chặn bởi số người
// dùng hoạt động trong một khoảng TTL vài giây, nên map không kịp lớn đến mức cần bộ quét riêng.
func (c *userCache) put(username string, user sqlc.User, now time.Time) {
	c.mu.RLock()
	ttl := c.ttl
	c.mu.RUnlock()

	if ttl <= 0 {
		return
	}

	if rdb := state.RedisClient; rdb != nil {
		raw, err := sonic.Marshal(fromUser(user))
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), cacheOpTimeout)
		defer cancel()
		// Ghi hỏng chỉ làm mất một lần tăng tốc, không làm sai kết quả — request kế tiếp đọc
		// lại từ PostgreSQL.
		_ = rdb.Set(ctx, userCacheKey(username), raw, ttl).Err()
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	c.entries[username] = userCacheEntry{user: user, expiresAt: now.Add(ttl)}
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
