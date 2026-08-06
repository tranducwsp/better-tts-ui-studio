package storage

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// Global là Store đang có hiệu lực, chốt một lần lúc khởi động qua Init.
//
// Một biến gói thay vì tham số truyền qua từng handler: đây là singleton thứ ba của repo,
// cùng lý do với GlobalTaskManager và GlobalManifestState — đổi nó thành dependency injection
// sẽ chạm vào mọi constructor handler, và việc đó đáng làm cùng lúc cho cả ba chứ không phải
// riêng cái này.
var Global Store

// baseDir là gốc lưu trữ đang có hiệu lực, giữ lại cho những chỗ còn cần đường dẫn thật.
var baseDir = "storage"

// Init chốt backend lưu trữ theo cấu hình.
//
// backend rỗng hoặc "local" dùng đĩa cục bộ. Tên khác được nhận diện ở đây để thông báo lỗi
// nói rõ giá trị nào hợp lệ, thay vì lặng lẽ rơi về local — một backend gõ sai mà vẫn khởi
// động được nghĩa là tệp đi vào chỗ không ai đọc, và điều đó chỉ lộ ra khi có người cần lại
// chúng.
func Init(backend, dir string, s3cfg S3Config) error {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", "local":
		if dir != "" {
			baseDir = dir
		}
		s, err := NewLocalStore(baseDir)
		if err != nil {
			return fmt.Errorf("không khởi tạo được kho cục bộ tại %q: %w", baseDir, err)
		}
		Global = s
		log.Printf("Lưu trữ: cục bộ tại %s", s.root)
		return nil

	case "s3":
		// Thời hạn cho lượt kiểm bucket lúc khởi động. Có hạn để một endpoint sai địa chỉ
		// biểu hiện thành lỗi rõ ràng thay vì một tiến trình treo mà không ai biết đang chờ gì.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		s, err := NewS3Store(ctx, s3cfg)
		if err != nil {
			return fmt.Errorf("không khởi tạo được kho S3 (bucket %q): %w", s3cfg.Bucket, err)
		}
		Global = s

		where := s3cfg.Endpoint
		if where == "" {
			where = "AWS " + s3cfg.Region
		}
		auth := "IAM role"
		if s3cfg.AccessKey != "" {
			auth = "khoá tĩnh"
		}
		log.Printf("Lưu trữ: S3 bucket %q tại %s (%s)", s3cfg.Bucket, where, auth)
		return nil

	default:
		return fmt.Errorf("STORAGE_BACKEND=%q không hợp lệ; chỉ nhận \"local\" hoặc \"s3\"", backend)
	}
}

// TempKey là khoá của âm thanh đã sinh cho một task.
func TempKey(taskID, format string) string {
	return "temp/" + safeSegment(taskID) + "." + safeSegment(format)
}

// TranscodeKey là khoá của bản đã chuyển mã, cất cạnh bản gốc.
//
// Hậu tố tách bằng dấu chấm để bộ quét dọn nhìn thấy chúng như mọi tệp tạm khác, và để vòng
// dò định dạng gốc không nhầm một bản chuyển mã là bản gốc.
func TranscodeKey(taskID, format string) string {
	return "temp/" + safeSegment(taskID) + ".to." + safeSegment(format)
}

// VoiceKey là khoá của tệp giọng tham chiếu người dùng đã lưu, tách theo mode rồi tới user.
func VoiceKey(modeID, userID, filename string) string {
	return safeSegment(modeID) + "/" + safeSegment(userID) + "/voice/" + safeSegment(filename)
}

// TempPrefix là nhánh chứa âm thanh tạm, thứ duy nhất bộ quét dọn được phép đụng vào.
const TempPrefix = "temp"

// Root trả về gốc lưu trữ cục bộ đang có hiệu lực.
//
// Chỉ dùng để nhận ra tiền tố trong những bản ghi cũ lưu đường dẫn hệ thống thay vì khoá.
// Với backend không phải đĩa, giá trị này không mô tả nơi thật sự chứa dữ liệu.
func Root() string { return baseDir }

// safeSegment thay mọi ký tự không nằm trong [A-Za-z0-9._-] bằng "_", và loại riêng ".."
// vì nó chỉ gồm các ký tự được phép nhưng vẫn trèo lên một cấp.
func safeSegment(s string) string {
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	out := []rune(s)
	for i, r := range out {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if !ok {
			out[i] = '_'
		}
	}
	return string(out)
}
