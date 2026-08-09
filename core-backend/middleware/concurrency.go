package middleware

import "net/http"

// ConcurrencyLimit giới hạn số request đang được xử lý đồng thời.
//
// Đây là chốt chặn *không phụ thuộc nội dung*: hạn mức body (BodyLimit, MAX_UPLOAD_SIZE_MB)
// và hạn mức RAM trong handler (handlers/utils.go) đều đo theo từng request, nhưng N request
// cùng lúc vẫn nhân RAM lên N lần. Với max cho trước, trần bộ nhớ của nhóm route là
// max × (RAM trần của một request), tính được trước và không bị điều khiển bởi input.
//
// Request vượt quá hạn chờ trong hàng đợi thay vì bị lỗi ngay — người dùng hợp lệ đi qua khi
// một chỗ trống, chậm hơn vài giây là chấp nhận được với best-effort như bóc chữ. Nếu client
// ngắt trong lúc chờ, request bỏ hàng không làm việc.
func ConcurrencyLimit(max int) func(http.Handler) http.Handler {
	sem := make(chan struct{}, max)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-r.Context().Done():
				// Client đã ngắt; không chiếm chỗ, không phục vụ.
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}