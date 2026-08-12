package parity_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"backend/types"
)

// Test cases shared with the frontend. To add new cases, edit
// docs/capability-resolution-cases.json and both sides pick it up — that is the point of
// keeping the data outside the source code.
const fixturePath = "../../docs/capability-resolution-cases.json"


type parityFixture struct {
	Manifest types.UniversalManifest `json:"manifest"`
	Cases    []struct {
		Why    string          `json:"why"`
		Mode   string          `json:"mode"`
		Expect map[string]bool `json:"expect"`
	} `json:"cases"`
	PlatformDefaults     map[string]bool `json:"platform_defaults"`
	DefaultMaxTextLength int             `json:"default_max_text_length"`
}

func loadFixture(t *testing.T) parityFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(fixturePath))
	if err != nil {
		t.Fatalf("cannot read shared test cases: %v", err)
	}
	var f parityFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("shared test cases are invalid: %v", err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("test cases are empty")
	}
	return f
}

func asMap(r types.ResolvedCapabilities) map[string]bool {
	return map[string]bool{
		"supports_preset_voices": r.SupportsPresetVoices,
		"supports_cloning":       r.SupportsCloning,
		"supports_voice_saving":  r.SupportsVoiceSaving,
		"supports_streaming":     r.SupportsStreaming,
		"supports_speed":         r.SupportsSpeed,
		"supports_pitch":         r.SupportsPitch,
		"supports_emotion":       r.SupportsEmotion,
		"supports_ssml":          r.SupportsSsml,
	}
}

// TestResolverMatchesSharedCases tests the Go resolver against the exact same cases the
// frontend uses. Two resolvers implement the same rules in two languages; no compiler
// constraint keeps them in sync, so this test is the only thing that catches drift.
func TestResolverMatchesSharedCases(t *testing.T) {
	f := loadFixture(t)

	for _, c := range f.Cases {
		got := asMap(f.Manifest.ResolveCapabilities(c.Mode))
		for key, want := range c.Expect {
			if got[key] != want {
				t.Errorf("mode %q (%s): %s = %v, want %v", c.Mode, c.Why, key, got[key], want)
			}
		}
	}
}

// TestPlatformDefaultsMatchFixture catches the case where someone changes the default
// constants on one side but forgets the other — the fixture is the shared record of the
// agreed-upon value.
func TestPlatformDefaultsMatchFixture(t *testing.T) {
	f := loadFixture(t)

	got := asMap(types.PlatformDefaultCapabilities)
	for key, want := range f.PlatformDefaults {
		if got[key] != want {
			t.Errorf("platform default %s = %v, fixture says %v", key, got[key], want)
		}
	}

	var nilManifest *types.UniversalManifest
	if nilManifest.ChunkSize() != f.DefaultMaxTextLength {
		t.Errorf("ChunkSize() with no manifest = %d, fixture says %d",
			nilManifest.ChunkSize(), f.DefaultMaxTextLength)
	}
}
