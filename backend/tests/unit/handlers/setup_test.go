package handlers_test

import (
	"os"
	"testing"

	"backend/storage"
)

// TestMain chuẩn bị storage.Global cho các bài kiểm thử handler cần truy cập kho.
//
// storage.Global bình thường được main.go chốt lúc khởi động, nên trong test nó là nil và
// handler đầu tiên chạm vào kho sẽ panic. Dựng một kho cục bộ trong thư mục tạm giữ các bài
// kiểm thử độc lập với nhau và không để lại gì trên máy.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "backend-handler-tests-*")
	if err != nil {
		panic("không tạo được thư mục kho tạm cho kiểm thử: " + err.Error())
	}

	if err := storage.Init("local", dir, storage.S3Config{}); err != nil {
		panic("không khởi tạo được kho cho kiểm thử: " + err.Error())
	}

	code := m.Run()

	os.RemoveAll(dir)
	os.Exit(code)
}
