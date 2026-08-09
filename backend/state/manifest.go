package state

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"backend/types"
)

// EngineManifestState quản lý lưu trữ RAM tập trung của Universal AI Engine Manifest.
type EngineManifestState struct {
	mu       sync.RWMutex
	manifest *types.UniversalManifest
}

// GlobalManifestState thể hiện trạng thái Singleton RAM Cache cho Manifest của AI Engine.
var GlobalManifestState = &EngineManifestState{}

// Set cập nhật bản Manifest mới vào bộ nhớ RAM Cache, sau khi kiểm tính nhất quán.
//
// Manifest tự mâu thuẫn bị từ chối và bản đang dùng được giữ nguyên: một lần reload lỗi
// không được phép hạ một Engine đang chạy tốt xuống trạng thái tệ hơn trước khi gọi. Những
// sai sót nhẹ hơn chỉ được ghi log — xem types.Validate để biết ranh giới giữa hai loại.
func (s *EngineManifestState) Set(m *types.UniversalManifest) error {
	errs, warnings := m.Validate()

	for _, w := range warnings {
		log.Printf("⚠️  Manifest: %s", w)
	}

	if len(errs) > 0 {
		for _, e := range errs {
			log.Printf("❌ Manifest bị từ chối: %v", e)
		}
		return fmt.Errorf("manifest không hợp lệ: %w", errors.Join(errs...))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifest = m
	return nil
}

// Clear xoá Manifest đang giữ, đưa nền tảng về trạng thái chưa phát hiện được Engine.
//
// Tách khỏi Set vì hai việc khác nhau: Set nhận một bản khai và phải kiểm nó, còn đây là chủ
// động quay về trạng thái rỗng. Trước đây cùng một hàm làm cả hai qua Set(nil), nên không
// phân biệt được "Engine khai sai" với "cố ý dọn trạng thái".
func (s *EngineManifestState) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifest = nil
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

// HasMode cho biết Engine có thực sự khai Mode này không.
//
// ResolveAudioSpec và ResolveCapabilities đều lặng lẽ quay về giá trị mặc định khi gặp
// mode lạ, nên chúng không dùng được để kiểm tra tính hợp lệ. Nơi nào lấy model_id từ
// client rồi đem đi ghép đường dẫn phải hỏi hàm này trước: một mode không tồn tại vừa là
// đường dẫn không truy vấn lại được, vừa là chỗ để chèn "../".
func (s *EngineManifestState) HasMode(mode string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.manifest == nil {
		return false
	}
	for i := range s.manifest.SupportedModes {
		if s.manifest.SupportedModes[i].ID == mode {
			return true
		}
	}
	return false
}

// errManifestUnavailable là câu trả lời chung khi chưa có Manifest để đối chiếu.
//
// app.BootstrapWeb/BootstrapWorker bắt buộc phải có Manifest trước khi cổng web hoặc hàng đợi mở, nên trạng thái này chỉ xảy ra nếu
// có ai đó xoá nó lúc đang chạy. Khi đó từ chối là lựa chọn duy nhất đúng: không biết Engine
// nhận gì thì không có cơ sở nào để nói một yêu cầu là hợp lệ.
var errManifestUnavailable = errors.New("chưa có manifest từ AI Engine, tạm thời không nhận yêu cầu tổng hợp")

// ValidatePitch kiểm tra Pitch theo đúng Mode được yêu cầu, vì cùng một Engine có thể có
// Mode hỗ trợ và Mode không. Truyền nil khi client không gửi Pitch.
func (s *EngineManifestState) ValidatePitch(pitch *float64, mode string) error {
	if pitch == nil {
		return nil
	}

	s.mu.RLock()
	m := s.manifest
	s.mu.RUnlock()

	// Không có Manifest thì không biết Mode này có nhận pitch không. Trước đây chỗ này trả
	// nil — tức mặc định là "có hỗ trợ", đúng chiều nguy hiểm hơn trong hai chiều đoán sai.
	if m == nil {
		return errManifestUnavailable
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
		return errManifestUnavailable
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
		return errManifestUnavailable
	}

	// Trần luôn có hiệu lực: TextLimit quay về mặc định nền tảng khi Engine khai 0, vì
	// "không khai" không đồng nghĩa với "không giới hạn".
	runeCount := len([]rune(text))
	if limit := m.TextLimit(); runeCount > limit {
		return fmt.Errorf("text length (%d characters) exceeds maximum limit (%d characters)", runeCount, limit)
	}

	if m.Constraints.SpeedRange.Min > 0 && speed < m.Constraints.SpeedRange.Min {
		return fmt.Errorf("speed %.2f is below minimum limit (%.2f)", speed, m.Constraints.SpeedRange.Min)
	}

	if m.Constraints.SpeedRange.Max > 0 && speed > m.Constraints.SpeedRange.Max {
		return fmt.Errorf("speed %.2f exceeds maximum limit (%.2f)", speed, m.Constraints.SpeedRange.Max)
	}

	// Mode luôn được kiểm: Set() từ chối manifest không khai mode nào, nên tới đây danh sách
	// chắc chắn không rỗng. Điều kiện len(...) > 0 trước đây chỉ che được đúng trường hợp
	// không nên che — một manifest rỗng thì mode nào cũng đi qua.
	if mode != "" {
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
