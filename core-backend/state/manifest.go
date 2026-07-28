package state

import (
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
