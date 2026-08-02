package handlers

import (
	"fmt"
	"net/http"

	"core-backend/state"
)

// Giới hạn kích thước upload, đặt một lần lúc khởi động từ MAX_UPLOAD_SIZE_MB.
//
// Trước đây ba handler đều viết cứng `32 << 20`, nên biến môi trường được đọc, được kiểm
// tra khoảng hợp lệ, được in ra log khởi động — rồi không ai thực thi. Đặt nó thành 8 hay
// 512 đều không thay đổi hành vi.
var maxUploadBytes int64 = 32 << 20

// SetMaxUploadMB đặt trần kích thước upload cho mọi handler nhận multipart.
func SetMaxUploadMB(mb int) {
	if mb > 0 {
		maxUploadBytes = int64(mb) << 20
	}
}

// MaxUploadBytes trả về trần hiện tại, tính bằng byte.
func MaxUploadBytes() int64 { return maxUploadBytes }

// parseUpload đọc form multipart trong giới hạn đã cấu hình.
//
// MaxBytesReader chặn ở tầng kết nối nên một tệp quá lớn bị ngắt ngay khi truyền, thay vì
// được nạp hết vào RAM rồi mới từ chối. Trả về thông báo nêu rõ giới hạn, vì "request quá
// lớn" mà không nói lớn hơn bao nhiêu thì người dùng không biết phải cắt bớt tới đâu.
func parseUpload(w http.ResponseWriter, r *http.Request) error {
	limit := uploadLimit()
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		return fmt.Errorf("tệp vượt quá giới hạn %d MB", limit>>20)
	}
	return nil
}

// uploadLimit ưu tiên giá trị Engine khai trong audio_spec.max_upload_bytes, vì chính
// Engine mới biết nó xử lý được tệp lớn tới đâu. MAX_UPLOAD_SIZE_MB là trần của hạ tầng,
// dùng khi Engine không khai — và luôn được tôn trọng nếu nó chặt hơn.
func uploadLimit() int64 {
	limit := maxUploadBytes
	if m := state.GlobalManifestState.Get(); m != nil && m.AudioSpec.MaxUploadBytes > 0 {
		if m.AudioSpec.MaxUploadBytes < limit {
			limit = m.AudioSpec.MaxUploadBytes
		}
	}
	return limit
}
