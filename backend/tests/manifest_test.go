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
		t.Fatalf("valid manifest was rejected: %v", err)
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

// TestValidate_FailsClosedWithoutManifest locks in the default direction of the validator.
//
// Previously all three validate functions returned nil when no Manifest was loaded, so while
// waiting for the Engine — or forever if the Engine never came up — every limit on text length,
// speed, pitch, emotion, and mode list had no effect. The UI enforces its own limits so nothing
// is visible through the UI; only direct API callers could bypass them.
func TestValidate_FailsClosedWithoutManifest(t *testing.T) {
	s := &state.EngineManifestState{}

	if err := s.ValidateRequest("any text at all", 1.0, "standard"); err == nil {
		t.Error("ValidateRequest must reject when no manifest is loaded")
	}

	pitch := 2.0
	if err := s.ValidatePitch(&pitch, "standard"); err == nil {
		t.Error("ValidatePitch must reject when no manifest is loaded")
	}

	emotion := "happy"
	if err := s.ValidateEmotion(&emotion, "standard"); err == nil {
		t.Error("ValidateEmotion must reject when no manifest is loaded")
	}

	// Not sending pitch/emotion is still valid: that means "let the Engine decide", not a value
	// that needs validation.
	if err := s.ValidatePitch(nil, "standard"); err != nil {
		t.Errorf("Nil pitch should not be rejected: %v", err)
	}
}

// TestValidate_TextLimitAppliesWhenEngineDeclaresZero: max_text_length = 0 is only a warning
// at manifest load time, so if the validator skips it when zero, the ceiling silently vanishes.
func TestValidate_TextLimitAppliesWhenEngineDeclaresZero(t *testing.T) {
	s := &state.EngineManifestState{}
	if err := s.Set(&types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
	}); err != nil {
		t.Fatalf("valid manifest was rejected: %v", err)
	}

	long := strings.Repeat("a", types.DefaultMaxTextLength+1)
	if err := s.ValidateRequest(long, 1.0, "standard"); err == nil {
		t.Errorf("text exceeding default %d characters must be rejected when engine declares 0",
			types.DefaultMaxTextLength)
	}

	ok := strings.Repeat("a", types.DefaultMaxTextLength)
	if err := s.ValidateRequest(ok, 1.0, "standard"); err != nil {
		t.Errorf("text exactly at the default ceiling must be accepted: %v", err)
	}
}
