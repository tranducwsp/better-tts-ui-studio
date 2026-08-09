package middleware

import (
	"net/http"
	"strings"
)

// maxJSONBodyBytes là trần body cho payload JSON/trường-text của API.
//
// Trước đây không có trần nào: Sonic đọc body tới EOF, một body bất kỳ lớn cỡ nào cũng bị
// dựng đầy trong RAM. 2 MiB thừa cho mọi request hợp lệ (text tổng hợp ≤3000 ký tự theo
// manifest, upload đã ngắt riêng) và đủ nhỏ để một kẻ gửi payload khổng lồ không làm cạn bộ
// nhớ backend.
const jsonBodyLimit = 2 << 20 // 2 MiB

// BodyLimit giới hạn body request xuống jsonBodyLimit.
//
// Multipart/form-data bị loại: upload giọng nói lên tới MAX_UPLOAD_SIZE_MB (default 256MB) và
// các handler upload đã tự đặt MaxBytesReader riêng — đè trần 2 MiB này lên chúng sẽ làm hỏng
// tính năng tải file.
func BodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := r.Header.Get("Content-Type"); !strings.HasPrefix(c, "multipart/") {
			r.Body = http.MaxBytesReader(w, r.Body, jsonBodyLimit)
		}
		next.ServeHTTP(w, r)
	})
}