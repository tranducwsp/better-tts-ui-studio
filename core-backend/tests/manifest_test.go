package tests

import (
	"sync"
	"testing"

	"core-backend/state"
	"core-backend/types"
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
			s.Set(manifest)
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
	s.Set(manifest)

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
