package state

import (
	"fmt"
	"sync"

	"core-backend/types"
)

// EngineManifestState quản lý lưu trữ RAM tập trung của Universal AI Engine Manifest.
type EngineManifestState struct {
	mu       sync.RWMutex
	manifest *types.UniversalManifest
}

// GlobalManifestState thể hiện trạng thái Singleton RAM Cache cho Manifest của AI Engine.
var GlobalManifestState = &EngineManifestState{}

// Set cập nhật bản Manifest mới vào bộ nhớ RAM Cache.
func (s *EngineManifestState) Set(m *types.UniversalManifest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifest = m
}

// Get truy xuất bản Manifest hiện tại từ RAM Cache (Thread-safe).
func (s *EngineManifestState) Get() *types.UniversalManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manifest
}

// IsLoaded kiểm tra xem Manifest đã được nạp thành công từ AI Engine chưa.
func (s *EngineManifestState) IsLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manifest != nil
}

// ValidateRequest thực hiện bẫy lỗi động dựa trên thông số Ràng buộc (Constraints) của Manifest.
func (s *EngineManifestState) ValidateRequest(text string, speed float64) error {
	s.mu.RLock()
	m := s.manifest
	s.mu.RUnlock()

	if m == nil {
		return nil
	}

	runeCount := len([]rune(text))
	if m.Constraints.MaxTextLength > 0 && runeCount > m.Constraints.MaxTextLength {
		return fmt.Errorf("độ dài văn bản (%d ký tự) vượt quá giới hạn tối đa (%d ký tự)", runeCount, m.Constraints.MaxTextLength)
	}

	if m.Constraints.SpeedRange.Min > 0 && speed < m.Constraints.SpeedRange.Min {
		return fmt.Errorf("tốc độ %.2f nhỏ hơn giới hạn tối thiểu (%.2f)", speed, m.Constraints.SpeedRange.Min)
	}

	if m.Constraints.SpeedRange.Max > 0 && speed > m.Constraints.SpeedRange.Max {
		return fmt.Errorf("tốc độ %.2f vượt quá giới hạn tối đa (%.2f)", speed, m.Constraints.SpeedRange.Max)
	}

	return nil
}
