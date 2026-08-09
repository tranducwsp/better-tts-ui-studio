package middleware

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"core-backend/db/sqlc"
	"core-backend/state"

	"github.com/redis/go-redis/v9"
)

// RateLimit giới hạn số request theo IP trong một cửa sổ thời gian trượt.
//
// scope tách khoá Redis giữa các nhóm route: register+login dùng chung một budget (dò mật
// khẩu / tạo rác), còn refresh phải có budget RIÊNG — nó bị frontend gọi trên mỗi lần tải
// trang, kể cả khi người dùng chưa đăng nhập, nên gộp chung với login sẽ để một người chỉ cần
// tải lại trang vài lần là làm cạn budget và chặn cả đăng nhập của chính họ. Khoá Redis không
// phân biệt limiter instance nào ghi, nên hai nhóm cùng scope ghi vào một khoá là một budget.
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
func RateLimit(scope string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return rateLimitBy(func(r *http.Request) string { return "ip:" + ClientIP(r) }, scope, limit, window)
}

// RateLimitUser giống RateLimit nhưng khoá theo user đã xác thực thay vì IP.
//
// Dùng cho route nặng (như /extract-text): một người dùng hợp lệ vẫn có thể quấy tiếp bằng cách
// giữ chu kỳ gọi; khoá theo user ID đếm đúng từng người, không bị ai đó dưới cùng NAT ăn hết
// budget chung.
func RateLimitUser(scope string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return rateLimitBy(func(r *http.Request) string {
		if u, ok := r.Context().Value(UserContextKey).(*sqlc.User); ok && u != nil && u.ID != "" {
			return "user:" + u.ID
		}
		return "ip:" + ClientIP(r)
	}, scope, limit, window)
}

// rateLimitBy là phần chung của RateLimit và RateLimitUser: keyFn quyết định danh tính người
// gọi, phần còn lại (cửa sổ trượt Redis, fallback RAM, reap) dùng chung.
func rateLimitBy(keyFn func(*http.Request) string, scope string, limit int, window time.Duration) func(http.Handler) http.Handler {
	l := &keyedLimiter{
		scope:  scope,
		limit:  limit,
		window: window,
		keyFn:  keyFn,
		hits:   make(map[string][]time.Time),
	}
	go l.reap()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(r.Context(), keyFn(r), time.Now()) {
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

type keyedLimiter struct {
	scope  string
	limit  int
	window time.Duration
	keyFn  func(*http.Request) string

	mu   sync.Mutex
	hits map[string][]time.Time

	// seq phân biệt hai request tới trong cùng một nano giây. Không có nó, ZADD lần sau ghi
	// đè member cũ thay vì thêm một lượt, và hạn mức đếm thiếu.
	seq atomic.Int64
}

// rateLimitKey đặt tiền tố để khoá không đụng khoá nào khác trên cùng Redis.
func rateLimitKey(scope, key string) string { return "ratelimit:" + scope + ":" + key }

// allow ghi nhận một lượt gọi và cho biết nó có nằm trong hạn mức không.
func (l *keyedLimiter) allow(ctx context.Context, key string, now time.Time) bool {
	if rdb := state.RedisClient; rdb != nil {
		// Không dùng context của request: nó bị huỷ khi client ngắt kết nối, và một người dò
		// mật khẩu tự ngắt sau mỗi lượt sẽ khiến lượt đó không được tính.
		opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheOpTimeout)
		defer cancel()

		res, err := slidingWindowScript.Run(opCtx, rdb,
			[]string{rateLimitKey(l.scope, key)},
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
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}

	l.hits[key] = append(kept, now)
	return true
}

// reap dọn các khoá đã hết dấu vết khỏi bộ đếm RAM.
//
// Không có nó, map giữ một khoá cho mỗi IP từng gọi tới và chỉ lớn lên — tức là một cách rẻ
// để làm cạn RAM của chính máy chủ mà lớp giới hạn này đang bảo vệ. Nhánh Redis không cần:
// EXPIRE trong script tự thu hồi khoá.
func (l *keyedLimiter) reap() {
	for range time.Tick(10 * time.Minute) {
		cutoff := time.Now().Add(-l.window)
		l.mu.Lock()
		for key, ts := range l.hits {
			fresh := false
			for _, t := range ts {
				if t.After(cutoff) {
					fresh = true
					break
				}
			}
			if !fresh {
				delete(l.hits, key)
			}
		}
		l.mu.Unlock()
	}
}

// trustedProxies là các dải mạng được phép đặt X-Forwarded-For.
//
// Rỗng nghĩa là không tin header đó bao giờ. Đọc-nhiều-ghi-một-lần: SetTrustedProxies chạy
// đúng một lần lúc khởi động, trước khi router nhận request đầu tiên.
var trustedProxies []*net.IPNet

// SetTrustedProxies nạp danh sách dải proxy tin cậy từ cấu hình.
//
// Mục không phân tích được sẽ làm dừng tiến trình: một dải viết sai chính tả âm thầm bị bỏ
// qua nghĩa là hạn mức khoá theo địa chỉ của proxy — mọi người dùng chung một khoá — hoặc
// theo một header giả mạo được. Cả hai đều là hỏng ngầm, và cấu hình sai thì phải thấy ngay
// lúc khởi động.
func SetTrustedProxies(cidrs []string) {
	trustedProxies = nil
	for _, c := range cidrs {
		// IP trần cũng nhận: một proxy duy nhất không cần viết thành /32.
		if !strings.Contains(c, "/") {
			if ip := net.ParseIP(c); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				c += "/" + strconv.Itoa(bits)
			}
		}
		_, network, err := net.ParseCIDR(c)
		if err != nil {
			log.Fatalf("TRUSTED_PROXIES: %q không phải CIDR hợp lệ: %v", c, err)
		}
		trustedProxies = append(trustedProxies, network)
	}
	if len(trustedProxies) > 0 {
		log.Printf("Tin X-Forwarded-For từ %d dải proxy", len(trustedProxies))
	}
}

// clientIP lấy địa chỉ người gọi, chỉ tin X-Forwarded-For khi kết nối đến từ proxy tin cậy.
//
// X-Forwarded-For do client đặt được. Trước đây header này được tin vô điều kiện, nên khoá
// hạn mức chính là một giá trị người gọi tự chọn: đổi header mỗi lần là hạn mức không còn tác
// dụng — đo được 200/200 request lọt qua một hạn mức 10/phút, tức là vượt hoàn toàn chứ không
// phải "một lớp làm chậm". Đó cũng là lớp duy nhất chắn việc dò mật khẩu và chắn việc bắt
// máy chủ băm bcrypt liên tục.
//
// Lấy phần tử CUỐI CÙNG mà vẫn còn nằm ngoài dải tin cậy, duyệt từ phải sang: các phần tử
// bên phải do proxy của ta ghi nên tin được, còn phần bên trái thì client đã có thể bịa sẵn
// trước khi request tới proxy.
func ClientIP(r *http.Request) string {
	remote := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}

	if len(trustedProxies) == 0 || !isTrustedProxy(remote) {
		return remote
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remote
	}

	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		if hop == "" {
			continue
		}
		if net.ParseIP(hop) == nil {
			// Giá trị không phải IP thì mọi thứ bên trái nó cũng không tin được nữa.
			break
		}
		if !isTrustedProxy(hop) {
			return hop
		}
	}
	return remote
}

// isTrustedProxy cho biết một địa chỉ có nằm trong dải proxy đã khai không.
func isTrustedProxy(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	for _, network := range trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
