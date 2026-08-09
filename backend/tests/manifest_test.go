package tests

import (
	"strings"
	"sync"
	"testing"

	"backend/state"
	"backend/types"
)

func TestEngineManifestState_ConcurrentAccess(t *testing.T) {
	s := &state.EngineManifestState{}

	manifest := &types.UniversalManifest{
		EngineID:   "concurrent-engine",
		EngineName: "Concurrent Test Engine",
		Constraints: types.EngineConstraints{
			MaxTextLength: 100,
			SpeedRange: types.RangeConstraint{
				Min: 0.5,
				Max: 2.0,
			},
		},
		SupportedModes: []types.EngineModeSpec{
			{ID: "standard"},
		},
	}

	var wg sync.WaitGroup

	// Writer goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Set(manifest)
		}()
	}

	// Reader goroutines
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Get()
			_ = s.IsLoaded()
			_ = s.ValidateRequest("Test string", 1.0, "standard")
		}()
	}

	wg.Wait()

	if !s.IsLoaded() {
		t.Errorf("Expected Manifest to be loaded after concurrent sets")
	}
}

func TestEngineManifestState_Validation(t *testing.T) {
	s := &state.EngineManifestState{}

	manifest := &types.UniversalManifest{
		Constraints: types.EngineConstraints{
			MaxTextLength: 10,
			SpeedRange: types.RangeConstraint{
				Min: 0.5,
				Max: 2.0,
			},
		},
		SupportedModes: []types.EngineModeSpec{
			{ID: "standard"},
		},
	}
	if err := s.Set(manifest); err != nil {
		t.Fatalf("manifest hợp lệ mà bị từ chối: %v", err)
	}

	// Test max text length error
	err := s.ValidateRequest("This text is way too long for max length 10", 1.0, "standard")
	if err == nil {
		t.Errorf("Expected error for text exceeding max length")
	}

	// Test speed below min
	err = s.ValidateRequest("Short text", 0.1, "standard")
	if err == nil {
		t.Errorf("Expected error for speed below min")
	}

	// Test speed above max
	err = s.ValidateRequest("Short text", 5.0, "standard")
	if err == nil {
		t.Errorf("Expected error for speed above max")
	}

	// Test invalid mode
	err = s.ValidateRequest("Short text", 1.0, "invalid-mode")
	if err == nil {
		t.Errorf("Expected error for invalid mode")
	}

	// Test valid request
	err = s.ValidateRequest("Short text", 1.0, "standard")
	if err != nil {
		t.Errorf("Expected no error for valid request, got: %v", err)
	}
}

// TestValidate_FailsClosedWithoutManifest khoá lại hướng mặc định của bộ kiểm tra.
//
// Trước đây cả ba hàm validate đều trả nil khi chưa có Manifest, nên trong lúc chờ Engine —
// hoặc mãi mãi nếu Engine không lên — mọi giới hạn về độ dài văn bản, speed, pitch, emotion
// và danh sách mode đều không có hiệu lực. Giao diện tự giới hạn nên qua UI không thấy gì;
// chỉ ai gọi thẳng API mới đi qua được.
func TestValidate_FailsClosedWithoutManifest(t *testing.T) {
	s := &state.EngineManifestState{}

	if err := s.ValidateRequest("bất kỳ văn bản nào", 1.0, "standard"); err == nil {
		t.Error("ValidateRequest phải từ chối khi chưa có manifest")
	}

	pitch := 2.0
	if err := s.ValidatePitch(&pitch, "standard"); err == nil {
		t.Error("ValidatePitch phải từ chối khi chưa có manifest")
	}

	emotion := "happy"
	if err := s.ValidateEmotion(&emotion, "standard"); err == nil {
		t.Error("ValidateEmotion phải từ chối khi chưa có manifest")
	}

	// Không gửi pitch/emotion vẫn là hợp lệ: đó là "để Engine tự quyết", không phải một giá
	// trị cần đối chiếu.
	if err := s.ValidatePitch(nil, "standard"); err != nil {
		t.Errorf("Pitch nil không nên bị từ chối: %v", err)
	}
}

// TestValidate_TextLimitAppliesWhenEngineDeclaresZero: max_text_length = 0 chỉ là cảnh báo
// lúc nạp manifest, nên nếu bộ kiểm tra bỏ qua khi nó bằng 0 thì trần biến mất trong im lặng.
func TestValidate_TextLimitAppliesWhenEngineDeclaresZero(t *testing.T) {
	s := &state.EngineManifestState{}
	if err := s.Set(&types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
	}); err != nil {
		t.Fatalf("manifest hợp lệ mà bị từ chối: %v", err)
	}

	long := strings.Repeat("a", types.DefaultMaxTextLength+1)
	if err := s.ValidateRequest(long, 1.0, "standard"); err == nil {
		t.Errorf("văn bản vượt mặc định %d ký tự phải bị từ chối khi engine khai 0",
			types.DefaultMaxTextLength)
	}

	ok := strings.Repeat("a", types.DefaultMaxTextLength)
	if err := s.ValidateRequest(ok, 1.0, "standard"); err != nil {
		t.Errorf("văn bản đúng bằng trần mặc định phải được nhận: %v", err)
	}
}
