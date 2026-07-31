package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// StorageProvider định nghĩa interface lưu trữ file âm thanh linh hoạt (Local / MinIO / S3).
type StorageProvider interface {
	SaveAudio(ctx context.Context, category string, fileName string, data []byte) (string, error)
	GetURL(ctx context.Context, filePath string) (string, error)
	DeleteAudio(ctx context.Context, filePath string) error
}

var globalProvider StorageProvider

// InitStorage khởi tạo Storage Provider mặc định (chế độ Local đĩa cứng).
func InitStorage(baseDir string) {
	globalProvider = NewLocalStorage(baseDir)
}

// GetStorage lấy Storage Provider toàn cục hiện tại.
func GetStorage() StorageProvider {
	if globalProvider == nil {
		globalProvider = NewLocalStorage("storage")
	}
	return globalProvider
}

// LocalStorage triển khai StorageProvider cho lưu trữ đĩa cứng cục bộ.
type LocalStorage struct {
	BaseDir string
}

// NewLocalStorage khởi tạo LocalStorage provider.
func NewLocalStorage(baseDir string) *LocalStorage {
	_ = os.MkdirAll(baseDir, 0755)
	return &LocalStorage{BaseDir: baseDir}
}

// SaveAudio lưu file đĩa local và trả về đường dẫn file.
func (l *LocalStorage) SaveAudio(ctx context.Context, category string, fileName string, data []byte) (string, error) {
	targetDir := filepath.Join(l.BaseDir, category)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create storage directory: %w", err)
	}

	fullPath := filepath.Join(targetDir, fileName)
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write audio file: %w", err)
	}

	return fullPath, nil
}

// GetURL trả về đường dẫn public để Frontend truy cập file audio (Ví dụ: /storage/temp/audio.wav).
func (l *LocalStorage) GetURL(ctx context.Context, filePath string) (string, error) {
	if filePath == "" {
		return "", nil
	}
	// Đối với LocalStorage, chuẩn hóa đường dẫn tương đối cho Web URL
	cleanPath := filepath.ToSlash(filePath)
	return "/" + cleanPath, nil
}

// DeleteAudio xóa file khỏi đĩa local.
func (l *LocalStorage) DeleteAudio(ctx context.Context, filePath string) error {
	if filePath == "" {
		return nil
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete audio file: %w", err)
	}
	return nil
}
