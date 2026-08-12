package types_test

import (
	"strings"
	"testing"

	"backend/state"
	"backend/types"
)

func boolPtr(b bool) *bool { return &b }

// A Manifest with no modes makes every subsequent resolver call guesswork, so this is a
// fatal error, not a warning.
func TestValidateRejectsManifestWithoutModes(t *testing.T) {
	m := &types.UniversalManifest{EngineID: "e", EngineName: "E"}

	errs, _ := m.Validate()
	if len(errs) == 0 {
		t.Fatal("manifest with no modes should have been rejected")
	}
}

func TestValidateRejectsDuplicateAndEmptyModeIDs(t *testing.T) {
	cases := []struct {
		name  string
		modes []types.EngineModeSpec
	}{
		{
			// The resolver stops at the first matching mode, so the second one is both invisible and
			// shows the author thought they were configuring something else.
			name: "duplicate id",
			modes: []types.EngineModeSpec{
				{ID: "standard", Name: "A"},
				{ID: "standard", Name: "B"},
			},
		},
		{
			name:  "empty id",
			modes: []types.EngineModeSpec{{ID: "", Name: "Nameless"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &types.UniversalManifest{SupportedModes: tc.modes}
			if errs, _ := m.Validate(); len(errs) == 0 {
				t.Error("should have been rejected")
			}
		})
	}
}

// A healthy manifest must pass cleanly, with no errors and no warnings — otherwise warnings
// become noise that operators learn to ignore.
func TestValidateAcceptsCoherentManifest(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{ID: "standard", Name: "Standard"},
			{
				ID:   "clone",
				Name: "Clone",
				Capabilities: types.EngineCapabilities{
					SupportsCloning:      boolPtr(true),
					SupportsPresetVoices: boolPtr(false),
				},
			},
		},
		AudioSpec: types.AudioSpec{
			SupportedFormats:      []string{"wav", "mp3"},
			SupportedSampleRates:  []int{24000},
			DefaultFormat:         "wav",
			DefaultSampleRate:     24000,
			ReferenceAudioFormats: []string{"wav"},
			MaxUploadBytes:        100 << 20,
			MaxReferenceBytes:     10 << 20,
		},
	}

	errs, warnings := m.Validate()
	if len(errs) > 0 {
		t.Errorf("healthy manifest has errors: %v", errs)
	}
	if len(warnings) > 0 {
		t.Errorf("healthy manifest has warnings: %v", warnings)
	}
}

// The contradictions below are resolvable, so they are only warnings — but they must warn,
// because each leads to a wrong behavior that the author cannot see at their declaration site.
func TestValidateWarnsOnResolvableContradictions(t *testing.T) {
	cases := []struct {
		name     string
		manifest *types.UniversalManifest
		expect   string
	}{
		{
			name: "default_format outside supported_formats",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				AudioSpec: types.AudioSpec{
					SupportedFormats: []string{"wav"},
					DefaultFormat:    "opus",
				},
			},
			expect: "default_format",
		},
		{
			name: "default_sample_rate outside supported_sample_rates",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				AudioSpec: types.AudioSpec{
					SupportedSampleRates: []int{16000},
					DefaultSampleRate:    48000,
				},
			},
			expect: "default_sample_rate",
		},
		{
			// The post-trim ceiling exceeding the original file ceiling makes the trim step meaningless.
			name: "max_reference_bytes exceeds max_upload_bytes",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				AudioSpec: types.AudioSpec{
					MaxUploadBytes:    5 << 20,
					MaxReferenceBytes: 50 << 20,
				},
			},
			expect: "max_reference_bytes",
		},
		{
			name: "model_sort points to nonexistent mode",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				UISchema:       &types.UISchemaSpec{ModelSort: []string{"standard", "nonexistent"}},
			},
			expect: "model_sort",
		},
		{
			name: "option_panel points to nonexistent mode",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				UISchema: &types.UISchemaSpec{
					OptionPanel: map[string]types.ModelOptionSpec{"nonexistent": {}},
				},
			},
			expect: "option_panel",
		},
		{
			// The list is declared but will never appear, so the author thinks it is done.
			name: "preset_voices on mode with preset voices disabled",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{
					ID:           "clone",
					Capabilities: types.EngineCapabilities{SupportsPresetVoices: boolPtr(false)},
				}},
				UISchema: &types.UISchemaSpec{
					OptionPanel: map[string]types.ModelOptionSpec{
						"clone": {PresetVoices: []types.PresetVoiceSpec{{ID: "v1", Name: "V1"}}},
					},
				},
			},
			expect: "preset_voices",
		},
		{
			name: "cloning mode did not declare reference_audio_formats",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{
					ID:           "clone",
					Capabilities: types.EngineCapabilities{SupportsCloning: boolPtr(true)},
				}},
				// Engine also did not declare, so there is nothing to inherit. The platform default only
				// applies at resolve time; here we want to know whether the Engine declared it itself.
				AudioSpec: types.AudioSpec{ReferenceAudioFormats: []string{}},
			},
			expect: "reference_audio_formats",
		},
		{
			name: "speed_range reversed",
			manifest: &types.UniversalManifest{
				SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
				Constraints: types.EngineConstraints{
					SpeedRange: types.RangeConstraint{Min: 2.0, Max: 0.5},
				},
			},
			expect: "speed_range",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs, warnings := tc.manifest.Validate()
			if len(errs) > 0 {
				t.Fatalf("should only be a warning, but was rejected: %v", errs)
			}
			joined := strings.Join(warnings, " | ")
			if !strings.Contains(joined, tc.expect) {
				t.Errorf("warning containing %q not found; received: %s", tc.expect, joined)
			}
		})
	}
}

// The most important property of load-time validation: a failed reload must not downgrade
// a running Engine to a worse state than before the call.
func TestSetKeepsPreviousManifestWhenRejected(t *testing.T) {
	s := &state.EngineManifestState{}

	good := &types.UniversalManifest{
		EngineID:       "good",
		SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
	}
	if err := s.Set(good); err != nil {
		t.Fatalf("valid manifest was rejected: %v", err)
	}

	// No modes at all: fatal.
	if err := s.Set(&types.UniversalManifest{EngineID: "broken"}); err == nil {
		t.Fatal("invalid manifest should have been rejected")
	}

	if got := s.Get(); got == nil || got.EngineID != "good" {
		t.Errorf("the current version must be preserved after a failed reload, got %+v", got)
	}
}

func TestSetRejectsNilAndClearResets(t *testing.T) {
	s := &state.EngineManifestState{}

	if err := s.Set(nil); err == nil {
		t.Error("Set(nil) should be an error; use Clear to reset state")
	}

	if err := s.Set(&types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{{ID: "standard"}},
	}); err != nil {
		t.Fatalf("valid manifest was rejected: %v", err)
	}
	if !s.IsLoaded() {
		t.Fatal("data preparation failed")
	}

	s.Clear()
	if s.IsLoaded() {
		t.Error("Clear must return the state to unloaded")
	}
}
