package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"core-backend/state"

	"github.com/redis/go-redis/v9"
)

// RateLimit giới hạn số request theo IP trong một cửa sổ thời gian trượt.
//
// Đặt trên /login và /register vì hai route đó là nơi một vòng lặp có giá trị với người
// ngoài: login không giới hạn là dò mật khẩu miễn phí, còn register không giới hạn vừa tạo
// rác trong bảng users vừa buộc máy chủ chạy một lượt bcrypt cho mỗi lần gọi — bcrypt
// DefaultCost tốn hàng chục ms CPU, nên chính lớp bảo vệ mật khẩu trở thành đòn bẩy để làm
// nghẽn máy.
//
// Cửa sổ trượt chứ không phải cửa sổ cố định: với cửa sổ cố định, người gọi dồn hết lượt vào
// cuối cửa sổ này và đầu cửa sổ sau sẽ đi được gấp đôi hạn mức trong khoảnh khắc giao nhau.
//
// Bộ đếm nằm trên Redis khi có, RAM tiến trình khi không. Với bộ đếm trong RAM, hạn mức thực
// tế là limit × số replica: mỗi tiến trình đếm riêng, nên người dò mật khẩu chỉ cần được load
// balancer rải đều là có thêm bấy nhiêu lần thử — và đó là trạng thái mặc định của một
// deployment nhiều node, không phải trường hợp hiếm.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	l := &ipLimiter{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
	}
	go l.reap()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(r.Context(), clientIP(r), time.Now()) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"detail":"Quá nhiều yêu cầu. Vui lòng thử lại sau."}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// slidingWindowScript đếm và quyết định trong MỘT lượt gọi Redis.
//
// Phải là script chứ không phải chuỗi lệnh rời: đọc số lượt rồi mới ghi thêm là hai bước, và
// hai request đồng thời đều đọc được "còn chỗ" trước khi bên nào kịp ghi — đúng lúc hạn mức
// cần chính xác nhất thì nó hở. Redis chạy script tuần tự nên không có khe đó.
//
// ZSET với score là thời điểm gọi: ZREMRANGEBYSCORE cắt phần đã rơi ra khỏi cửa sổ, nên đây
// là cửa sổ trượt thật, không phải cửa sổ cố định.
//
// KEYS[1] khoá  ARGV[1] mốc cắt  ARGV[2] thời điểm hiện tại  ARGV[3] hạn mức  ARGV[4] TTL giây
// ARGV[5] chuỗi phân biệt request, để hai lượt cùng micro giây không ghi đè nhau
var slidingWindowScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
local used = redis.call('ZCARD', KEYS[1])
if used >= tonumber(ARGV[3]) then
  return 0
end
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[5])
redis.call('EXPIRE', KEYS[1], ARGV[4])
return 1
`)

type ipLimiter struct {
	limit  int
	window time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time

	// seq phân biệt hai request tới trong cùng một nano giây. Không có nó, ZADD lần sau ghi
	// đè member cũ thay vì thêm một lượt, và hạn mức đếm thiếu.
	seq atomic.Int64
}

// rateLimitKey đặt tiền tố để khoá không đụng khoá nào khác trên cùng Redis.
func rateLimitKey(ip string) string { return "ratelimit:ip:" + ip }

// allow ghi nhận một lượt gọi và cho biết nó có nằm trong hạn mức không.
func (l *ipLimiter) allow(ctx context.Context, ip string, now time.Time) bool {
	if rdb := state.RedisClient; rdb != nil {
		// Không dùng context của request: nó bị huỷ khi client ngắt kết nối, và một người dò
		// mật khẩu tự ngắt sau mỗi lượt sẽ khiến lượt đó không được tính.
		opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheOpTimeout)
		defer cancel()

		res, err := slidingWindowScript.Run(opCtx, rdb,
			[]string{rateLimitKey(ip)},
			now.Add(-l.window).UnixNano(),
			now.UnixNano(),
			l.limit,
			int(l.window.Seconds())+1,
			strconv.FormatInt(now.UnixNano(), 10)+"-"+strconv.FormatInt(l.seq.Add(1), 10),
		).Int64()

		if err == nil {
			return res == 1
		}
		// Redis lỗi thì rơi xuống bộ đếm RAM bên dưới. Fail-open hoàn toàn sẽ biến một sự cố
		// Redis thành cửa mở cho việc dò mật khẩu; bộ đếm mỗi tiến trình lỏng hơn nhưng vẫn
		// chặn, nên đó là lựa chọn đúng hơn cả hai thái cực.
	}

	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Lọc tại chỗ để không cấp phát lại slice cho mỗi request.
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= l.limit {
		l.hits[ip] = kept
		return false
	}

	l.hits[ip] = append(kept, now)
	return true
}

// reap dọn các IP đã hết dấu vết khỏi bộ đếm RAM.
//
// Không có nó, map giữ một khoá cho mỗi IP từng gọi tới và chỉ lớn lên — tức là một cách rẻ
// để làm cạn RAM của chính máy chủ mà lớp giới hạn này đang bảo vệ. Nhánh Redis không cần:
// EXPIRE trong script tự thu hồi khoá.
func (l *ipLimiter) reap() {
	for range time.Tick(10 * time.Minute) {
		cutoff := time.Now().Add(-l.window)
		l.mu.Lock()
		for ip, ts := range l.hits {
			fresh := false
			for _, t := range ts {
				if t.After(cutoff) {
					fresh = true
					break
				}
			}
			if !fresh {
				delete(l.hits, ip)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP lấy địa chỉ người gọi, ưu tiên X-Forwarded-For khi chạy sau proxy.
//
// Lấy phần tử ĐẦU của chuỗi X-Forwarded-For: đó là địa chỉ do proxy ngoài cùng ghi lại.
// Header này do client đặt được, nên khi không có proxy phía trước thì nó là thứ giả mạo
// được — hạn mức vẫn nên coi là lớp làm chậm, không phải lớp chặn tuyệt đối.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i != -1 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
