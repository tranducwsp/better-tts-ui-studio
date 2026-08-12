package types_test

import (
	"testing"

	"backend/types"
)

// TestResolveAudioSpecModeBeatsEngine verifies the Mode → Engine → Platform-Default
// resolution chain. A mode-level value must win over the engine-wide value, which wins
// over the platform default.
func TestResolveAudioSpecModeBeatsEngine(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{
				ID: "standard", Name: "Standard",
				AudioSpec: types.AudioSpec{
					DefaultFormat: "mp3",
				},
			},
		},
		AudioSpec: types.AudioSpec{
			SupportedFormats:      []string{"wav", "mp3"},
			SupportedSampleRates:  []int{24000},
			DefaultFormat:         "wav",
			DefaultSampleRate:     24000,
			ReferenceAudioFormats: []string{"wav"},
			ReferenceAudioSeconds: 5.0,
			MaxUploadBytes:        100 << 20,
			MaxReferenceBytes:     10 << 20,
		},
	}

	spec := m.ResolveAudioSpec("standard")

	if spec.DefaultFormat != "mp3" {
		t.Errorf("mode should override engine default_format: got %q, want mp3", spec.DefaultFormat)
	}
	if spec.DefaultSampleRate != 24000 {
		t.Errorf("mode inherits engine default_sample_rate: got %d, want 24000", spec.DefaultSampleRate)
	}
	if spec.MaxUploadBytes != 100<<20 {
		t.Errorf("mode inherits engine max_upload_bytes: got %d, want %d", spec.MaxUploadBytes, 100<<20)
	}
}

// TestResolveAudioSpecEngineFallsBackToPlatform verifies that when neither mode nor engine
// states a value, the platform default is used.
func TestResolveAudioSpecEngineFallsBackToPlatform(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{ID: "bare", Name: "Bare"},
		},
		// No AudioSpec at engine level — everything comes from the platform default.
	}

	spec := m.ResolveAudioSpec("bare")

	if spec.DefaultFormat != "wav" {
		t.Errorf("expected platform default format 'wav', got %q", spec.DefaultFormat)
	}
	if spec.DefaultSampleRate != 24000 {
		t.Errorf("expected platform default sample rate 24000, got %d", spec.DefaultSampleRate)
	}
	if len(spec.SupportedFormats) != 1 || spec.SupportedFormats[0] != "wav" {
		t.Errorf("expected platform default supported_formats [wav], got %v", spec.SupportedFormats)
	}
	if spec.MaxUploadBytes != 100<<20 {
		t.Errorf("expected platform default max_upload_bytes %d, got %d", 100<<20, spec.MaxUploadBytes)
	}
}

// TestResolveAudioSpecPartialModeOverride verifies that a mode can override only some fields,
// leaving others to inherit from the engine.
func TestResolveAudioSpecPartialModeOverride(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{
				ID: "fast", Name: "Fast",
				AudioSpec: types.AudioSpec{
					SupportedFormats: []string{"mp3"},
					DefaultFormat:    "mp3",
					// DefaultSampleRate, MaxUploadBytes etc. are NOT set — inherit.
				},
			},
		},
		AudioSpec: types.AudioSpec{
			SupportedFormats:     []string{"wav", "mp3", "flac"},
			SupportedSampleRates: []int{24000, 48000},
			DefaultFormat:        "wav",
			DefaultSampleRate:    24000,
			MaxUploadBytes:       50 << 20,
		},
	}

	spec := m.ResolveAudioSpec("fast")

	// Inherited from mode
	if spec.DefaultFormat != "mp3" {
		t.Errorf("mode override default_format: got %q, want mp3", spec.DefaultFormat)
	}
	// Inherited from engine
	if spec.DefaultSampleRate != 24000 {
		t.Errorf("inherit engine default_sample_rate: got %d, want 24000", spec.DefaultSampleRate)
	}
	if spec.MaxUploadBytes != 50<<20 {
		t.Errorf("inherit engine max_upload_bytes: got %d, want %d", spec.MaxUploadBytes, 50<<20)
	}
	// Mode supported_formats overrides engine entirely (not a merge)
	if len(spec.SupportedFormats) != 1 || spec.SupportedFormats[0] != "mp3" {
		t.Errorf("mode supported_formats overrides engine: got %v, want [mp3]", spec.SupportedFormats)
	}
}

// TestResolveAudioSpecUnknownMode returns engine-wide defaults when the mode is not found.
func TestResolveAudioSpecUnknownMode(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{ID: "standard", Name: "Standard"},
		},
		AudioSpec: types.AudioSpec{
			SupportedFormats:      []string{"wav"},
			SupportedSampleRates:  []int{24000},
			DefaultFormat:         "wav",
			DefaultSampleRate:     24000,
			ReferenceAudioFormats: []string{"wav"},
			ReferenceAudioSeconds: 5.0,
			MaxUploadBytes:        100 << 20,
			MaxReferenceBytes:     10 << 20,
		},
	}

	spec := m.ResolveAudioSpec("no_such_mode")

	if spec.DefaultFormat != "wav" {
		t.Errorf("unknown mode falls back to engine: got %q, want wav", spec.DefaultFormat)
	}
}

// TestResolveAudioSpecNilManifestReturnsPlatformDefaults verifies that a nil manifest
// returns the platform defaults without panicking.
func TestResolveAudioSpecNilManifestReturnsPlatformDefaults(t *testing.T) {
	var m *types.UniversalManifest

	spec := m.ResolveAudioSpec("anything")

	if spec.DefaultFormat != "wav" {
		t.Errorf("nil manifest returns platform default: got %q, want wav", spec.DefaultFormat)
	}
	if spec.MaxUploadBytes != 100<<20 {
		t.Errorf("nil manifest returns platform default: got %d, want %d", spec.MaxUploadBytes, 100<<20)
	}
}

// TestResolveAudioSpecAllFields verifies every field of the resolved AudioSpec matches
// the expected chain for a mode that overrides everything.
func TestResolveAudioSpecAllFields(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{
				ID: "full", Name: "Full",
				AudioSpec: types.AudioSpec{
					SupportedFormats:      []string{"flac", "wav"},
					SupportedSampleRates:  []int{48000},
					DefaultFormat:         "flac",
					DefaultSampleRate:     48000,
					ReferenceAudioFormats: []string{"flac", "wav"},
					ReferenceAudioSeconds: 10.0,
					MaxUploadBytes:        200 << 20,
					MaxReferenceBytes:     20 << 20,
				},
			},
		},
		AudioSpec: types.AudioSpec{
			SupportedFormats:      []string{"wav"},
			SupportedSampleRates:  []int{24000},
			DefaultFormat:         "wav",
			DefaultSampleRate:     24000,
			ReferenceAudioFormats: []string{"wav"},
			ReferenceAudioSeconds: 5.0,
			MaxUploadBytes:        100 << 20,
			MaxReferenceBytes:     10 << 20,
		},
	}

	spec := m.ResolveAudioSpec("full")

	if len(spec.SupportedFormats) != 2 || spec.SupportedFormats[0] != "flac" {
		t.Errorf("SupportedFormats: got %v, want [flac wav]", spec.SupportedFormats)
	}
	if len(spec.SupportedSampleRates) != 1 || spec.SupportedSampleRates[0] != 48000 {
		t.Errorf("SupportedSampleRates: got %v, want [48000]", spec.SupportedSampleRates)
	}
	if spec.DefaultFormat != "flac" {
		t.Errorf("DefaultFormat: got %q, want flac", spec.DefaultFormat)
	}
	if spec.DefaultSampleRate != 48000 {
		t.Errorf("DefaultSampleRate: got %d, want 48000", spec.DefaultSampleRate)
	}
	if len(spec.ReferenceAudioFormats) != 2 || spec.ReferenceAudioFormats[0] != "flac" {
		t.Errorf("ReferenceAudioFormats: got %v, want [flac wav]", spec.ReferenceAudioFormats)
	}
	if spec.ReferenceAudioSeconds != 10.0 {
		t.Errorf("ReferenceAudioSeconds: got %f, want 10.0", spec.ReferenceAudioSeconds)
	}
	if spec.MaxUploadBytes != 200<<20 {
		t.Errorf("MaxUploadBytes: got %d, want %d", spec.MaxUploadBytes, 200<<20)
	}
	if spec.MaxReferenceBytes != 20<<20 {
		t.Errorf("MaxReferenceBytes: got %d, want %d", spec.MaxReferenceBytes, 20<<20)
	}
}

// TestResolveAudioSpecEmptyValuesDoNotOverride verifies that zero/empty values in a mode's
// AudioSpec do NOT override the engine's non-zero values — the resolution helpers use
// "len > 0" / "> 0" / "!= \"\"" checks.
func TestResolveAudioSpecEmptyValuesDoNotOverride(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{
				ID: "empty", Name: "Empty",
				AudioSpec: types.AudioSpec{
					// All fields are zero/empty/nil — should not override engine.
				},
			},
		},
		AudioSpec: types.AudioSpec{
			SupportedFormats:      []string{"wav", "mp3"},
			SupportedSampleRates:  []int{24000, 48000},
			DefaultFormat:         "wav",
			DefaultSampleRate:     24000,
			ReferenceAudioFormats: []string{"wav", "mp3"},
			ReferenceAudioSeconds: 5.0,
			MaxUploadBytes:        100 << 20,
			MaxReferenceBytes:     10 << 20,
		},
	}

	spec := m.ResolveAudioSpec("empty")

	if spec.DefaultFormat != "wav" {
		t.Errorf("empty mode default_format should inherit engine: got %q, want wav", spec.DefaultFormat)
	}
	if len(spec.SupportedFormats) != 2 {
		t.Errorf("empty mode supported_formats should inherit engine: got %v, want [wav mp3]", spec.SupportedFormats)
	}
	if spec.MaxUploadBytes != 100<<20 {
		t.Errorf("empty mode max_upload_bytes should inherit engine: got %d, want %d", spec.MaxUploadBytes, 100<<20)
	}
}

// TestResolveAudioSpecNoEngineValues verifies that when the engine has no AudioSpec at all,
// the mode's values are resolved against the platform defaults.
func TestResolveAudioSpecNoEngineValues(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{
				ID: "custom", Name: "Custom",
				AudioSpec: types.AudioSpec{
					DefaultFormat:    "flac",
					MaxUploadBytes:   50 << 20,
					MaxReferenceBytes: 5 << 20,
				},
			},
		},
		// No engine-level AudioSpec — everything falls through to platform defaults.
	}

	spec := m.ResolveAudioSpec("custom")

	if spec.DefaultFormat != "flac" {
		t.Errorf("mode override default_format: got %q, want flac", spec.DefaultFormat)
	}
	if spec.DefaultSampleRate != 24000 {
		t.Errorf("no engine → platform default: got %d, want 24000", spec.DefaultSampleRate)
	}
	if spec.MaxUploadBytes != 50<<20 {
		t.Errorf("mode override max_upload_bytes: got %d, want %d", spec.MaxUploadBytes, 50<<20)
	}
	if spec.MaxReferenceBytes != 5<<20 {
		t.Errorf("mode override max_reference_bytes: got %d, want %d", spec.MaxReferenceBytes, 5<<20)
	}
}