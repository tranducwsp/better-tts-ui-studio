package tests

import (
	"os"
	"testing"

	"core-backend/storage"
)

// TestMain chuẩn bị những singleton mà handler cần trước khi bất kỳ bài kiểm thử nào chạy.
//
// storage.Global bình thường được main.go chốt lúc khởi động, nên trong test nó là nil và
// handler đầu tiên chạm vào kho sẽ panic. Dựng một kho cục bộ trong thư mục tạm giữ các bài
// kiểm thử độc lập với nhau và không để lại gì trên máy.
//
// Đây cũng là cái giá của việc storage.Global là biến gói: nếu Store được truyền vào từng
// handler thì mỗi bài kiểm thử tự cấp bản của mình và không cần bước này. Ba singleton của
// repo (Global, GlobalTaskManager, GlobalManifestState) nên được gỡ cùng một lượt, không
// phải riêng cái này.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "core-backend-tests-*")
	if err != nil {
		panic("không tạo được thư mục kho tạm cho kiểm thử: " + err.Error())
	}

	if err := storage.Init("local", dir); err != nil {
		panic("không khởi tạo được kho cho kiểm thử: " + err.Error())
	}

	code := m.Run()

	os.RemoveAll(dir)
	os.Exit(code)
}
