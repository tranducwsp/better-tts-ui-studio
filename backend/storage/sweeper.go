package storage

import (
	"context"
	"time"
)

// SweepTempObjects xoá âm thanh tạm cũ hơn retention và trả về số đối tượng đã xoá cùng số
// byte giải phóng.
//
// Đây là thao tác dữ liệu thuần trên Store — nó không tự chạy định kỳ, không giữ goroutine
// hay ticker. Vòng đời của tiến trình thuộc về package cron, nơi có quyền quyết định chạy
// bao lâu một lần và xử lý kết quả.
//
// Chỉ quét nhánh temp: giọng người dùng đã lưu nằm ở nhánh khác và không được phép xoá. Ràng
// buộc đó do List thi hành (không đệ quy), không phải do người gọi nhớ.
func SweepTempObjects(ctx context.Context, store Store, retention time.Duration) (int, int64, error) {
	audioObjs, err := store.List(ctx, AudioPrefix)
	if err != nil {
		audioObjs = nil
	}
	tempObjs, err := store.List(ctx, TempPrefix)
	if err != nil {
		tempObjs = nil
	}
	objects := append(audioObjs, tempObjs...)

	cutoff := time.Now().Add(-retention).Unix()
	var removed int
	var freed int64

	for _, o := range objects {
		if o.Modified > cutoff {
			continue
		}
		if err := store.Delete(ctx, o.Key); err != nil {
			// Tệp có thể đang được đọc để trả về cho client; lần quét sau sẽ dọn.
			continue
		}
		removed++
		freed += o.Size
	}

	return removed, freed, nil
}
