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

// ValidatePitch kiểm tra Pitch theo đúng Mode được yêu cầu, vì cùng một Engine có thể có
// Mode hỗ trợ và Mode không. Truyền nil khi client không gửi Pitch.
func (s *EngineManifestState) ValidatePitch(pitch *float64, mode string) error {
	if pitch == nil {
		return nil
	}

	s.mu.RLock()
	m := s.manifest
	s.mu.RUnlock()

	if m == nil {
		return nil
	}

	if !m.ResolveCapabilities(mode).SupportsPitch {
		return fmt.Errorf("pitch control is not supported by mode '%s'", mode)
	}

	r := m.Constraints.PitchRange
	if r.Min != 0 || r.Max != 0 {
		if *pitch < r.Min || *pitch > r.Max {
			return fmt.Errorf("pitch %.2f is outside the supported range (%.2f to %.2f)", *pitch, r.Min, r.Max)
		}
	}

	return nil
}

// ValidateEmotion kiểm tra Emotion theo Mode và đối chiếu SupportedEmotions của Manifest.
// Chuỗi rỗng nghĩa là "để Engine tự quyết" nên luôn hợp lệ.
func (s *EngineManifestState) ValidateEmotion(emotion *string, mode string) error {
	if emotion == nil || *emotion == "" {
		return nil
	}

	s.mu.RLock()
	m := s.manifest
	s.mu.RUnlock()

	if m == nil {
		return nil
	}

	if !m.ResolveCapabilities(mode).SupportsEmotion {
		return fmt.Errorf("emotion control is not supported by mode '%s'", mode)
	}

	for _, e := range m.Constraints.SupportedEmotions {
		if e == *emotion {
			return nil
		}
	}

	return fmt.Errorf("emotion '%s' is not supported by AI Engine", *emotion)
}

// ValidateRequest thực hiện bẫy lỗi động dựa trên thông số Ràng buộc (Constraints) và SupportedModes của Manifest.
func (s *EngineManifestState) ValidateRequest(text string, speed float64, mode string) error {
	s.mu.RLock()
	m := s.manifest
	s.mu.RUnlock()

	if m == nil {
		return nil
	}

	runeCount := len([]rune(text))
	if m.Constraints.MaxTextLength > 0 && runeCount > m.Constraints.MaxTextLength {
		return fmt.Errorf("text length (%d characters) exceeds maximum limit (%d characters)", runeCount, m.Constraints.MaxTextLength)
	}

	if m.Constraints.SpeedRange.Min > 0 && speed < m.Constraints.SpeedRange.Min {
		return fmt.Errorf("speed %.2f is below minimum limit (%.2f)", speed, m.Constraints.SpeedRange.Min)
	}

	if m.Constraints.SpeedRange.Max > 0 && speed > m.Constraints.SpeedRange.Max {
		return fmt.Errorf("speed %.2f exceeds maximum limit (%.2f)", speed, m.Constraints.SpeedRange.Max)
	}

	if mode != "" && len(m.SupportedModes) > 0 {
		validMode := false
		for _, sm := range m.SupportedModes {
			if sm.ID == mode {
				validMode = true
				break
			}
		}
		if !validMode {
			return fmt.Errorf("engine mode '%s' is not supported by AI Engine", mode)
		}
	}

	return nil
}
