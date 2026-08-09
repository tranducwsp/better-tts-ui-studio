package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound là câu trả lời chung khi một khoá không tồn tại.
//
// Có kiểu lỗi riêng vì người gọi cần phân biệt "chưa có" với "hỏng": task vừa tạo chưa có
// âm thanh là chuyện bình thường và trả 404, còn S3 từ chối quyền là sự cố và phải hiện ra.
// Không có nó, người gọi sẽ so sánh chuỗi lỗi, mà chuỗi của os và của S3 không giống nhau.
var ErrNotFound = errors.New("storage: không tìm thấy đối tượng")

// Store là nơi nền tảng cất tệp nhị phân.
//
// Tồn tại để chỗ lưu trữ trở thành một lựa chọn triển khai chứ không phải một giả định nằm
// rải rác trong handler. Trước đây mỗi handler tự gọi os.WriteFile và tự ghép đường dẫn, nên
// "để tệp ở đâu" bị viết lại ở năm chỗ và không đổi được nếu không sửa cả năm.
//
// Khoá là đường dẫn tương đối dùng dấu "/" (ví dụ "temp/<task>.mp3"), giống hình dạng khoá
// của S3. Bản local tự đổi sang dấu phân cách của hệ điều hành; người gọi không cần biết.
//
// Mọi phương thức nhận Context vì bản S3 là I/O qua mạng và phải huỷ được. Bản local bỏ qua
// nó, nhưng người gọi thì viết một kiểu cho cả hai.
type Store interface {
	// Put ghi nội dung từ src vào khoá. Chấp nhận io.Reader thay vì []byte để người gọi có thể
	// stream một tệp lớn từ đĩa/khác xuống mà không dựng toàn bộ trong RAM (đường upload giọng
	// nói trước đây đọc hết tệp vào bộ nhớ chỉ để chuyển vào đây).
	Put(ctx context.Context, key string, src io.Reader) error

	// Get đọc toàn bộ nội dung. Trả ErrNotFound nếu khoá không tồn tại.
	Get(ctx context.Context, key string) ([]byte, error)

	// Open mở luồng đọc, dành cho tệp lớn không nên nạp hết vào RAM.
	// Người gọi có trách nhiệm đóng. Trả ErrNotFound nếu khoá không tồn tại.
	Open(ctx context.Context, key string) (io.ReadCloser, error)

	// Exists cho biết khoá có tồn tại không, không tải nội dung về.
	Exists(ctx context.Context, key string) (bool, error)

	// Delete xoá khoá. Khoá không tồn tại KHÔNG phải lỗi: người gọi muốn nó biến mất, và nó
	// đã biến mất — bắt họ tự xử lý ErrNotFound ở đây chỉ tạo ra mã lặp lại ở mọi chỗ xoá.
	Delete(ctx context.Context, key string) error

	// List liệt kê các khoá bắt đầu bằng prefix, kèm thời điểm sửa và kích thước, để bộ quét
	// dọn quyết định xoá gì mà không phải tải nội dung.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// Presigner là một Store biết phát URL tải trực tiếp có thời hạn.
//
// Interface tuỳ chọn, tách khỏi Store vì không phải kho nào cũng làm được: bản đĩa cục bộ
// không có URL nào để phát. Người gọi kiểm bằng type assertion, nên thêm một backend không
// hỗ trợ presign về sau không phải viết một phương thức trả lỗi cho có.
//
// Kiểu này cũng là lý do handler không cần so sánh STORAGE_BACKEND với chuỗi "s3": nó hỏi
// "kho này phát URL được không" thay vì "kho này tên gì", nên thêm backend mới không phải
// sửa handler.
type Presigner interface {
	// PresignGet trả về URL tải trực tiếp, hết hiệu lực sau ttl.
	//
	// Không kiểm khoá có tồn tại: ký là phép tính cục bộ, còn kiểm tồn tại là một lượt gọi
	// mạng. Người gọi biết rõ hơn liệu có cần kiểm hay không.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// ObjectInfo là phần siêu dữ liệu bộ quét dọn cần.
type ObjectInfo struct {
	Key      string
	Size     int64
	Modified int64 // Unix seconds
}
