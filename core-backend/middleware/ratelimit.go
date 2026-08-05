package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
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
// Bộ đếm nằm trong RAM của từng tiến trình. Chạy nhiều replica thì hạn mức thực tế là
// limit × số replica — vẫn chặn được dò mật khẩu tự động, nhưng nếu cần con số chính xác thì
// phải chuyển sang Redis.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	l := &ipLimiter{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
	}
	go l.reap()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(clientIP(r), time.Now()) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconvItoa(int(window.Seconds())))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"detail":"Quá nhiều yêu cầu. Vui lòng thử lại sau."}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type ipLimiter struct {
	limit  int
	window time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time
}

// allow ghi nhận một lượt gọi và cho biết nó có nằm trong hạn mức không.
func (l *ipLimiter) allow(ip string, now time.Time) bool {
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

// reap dọn các IP đã hết dấu vết.
//
// Không có nó, map giữ một khoá cho mỗi IP từng gọi tới và chỉ lớn lên — tức là một cách rẻ
// để làm cạn RAM của chính máy chủ mà lớp giới hạn này đang bảo vệ.
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

// strconvItoa tránh import strconv chỉ để đổi một số giây thành chuỗi.
func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
