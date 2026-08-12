package tests

import (
	"testing"

	"backend/types"
)

func b(v bool) *bool { return &v }

func TestResolveCapabilities(t *testing.T) {
	m := &types.UniversalManifest{
		SupportedModes: []types.EngineModeSpec{
			{ID: "fast"},
			{ID: "emotion_v2", Capabilities: types.EngineCapabilities{
				SupportsPitch: b(true), SupportsEmotion: b(true)}},
			{ID: "locked", Capabilities: types.EngineCapabilities{
				SupportsStreaming: b(false)}},
		},
		Capabilities: types.EngineCapabilities{
			SupportsPitch: b(false), SupportsEmotion: b(false), SupportsStreaming: b(true),
		},
	}

	cases := []struct {
		mode                      string
		pitch, emotion, streaming bool
	}{
		{"fast", false, false, true},     // inherits all
		{"emotion_v2", true, true, true}, // mode overrides to true
		{"locked", false, false, false},  // mode overrides to false even though engine says true
		{"unknown", false, false, true},  // unknown mode -> engine-wide
		{"", false, false, true},         // empty -> engine-wide
	}
	for _, c := range cases {
		got := m.ResolveCapabilities(c.mode)
		if got.SupportsPitch != c.pitch || got.SupportsEmotion != c.emotion || got.SupportsStreaming != c.streaming {
			t.Errorf("%q: pitch=%v emotion=%v streaming=%v; want %v/%v/%v",
				c.mode, got.SupportsPitch, got.SupportsEmotion, got.SupportsStreaming, c.pitch, c.emotion, c.streaming)
		}
	}

	// nil manifest does not panic
	var nilM *types.UniversalManifest
	if r := nilM.ResolveCapabilities("x"); !r.SupportsStreaming {
		t.Error("nil manifest should default to streaming=true")
	}
}

func TestChunkSize(t *testing.T) {
	cases := []struct{ maxText, pref, want int }{
		{3000, 1000, 1000}, // pref is smaller -> use pref
		{500, 9999, 500},   // pref larger than ceiling -> clamp to ceiling
		{500, 0, 500},      // not declared -> ceiling
		{0, 1000, 3000},    // engine declares nothing -> default
	}
	for _, c := range cases {
		m := &types.UniversalManifest{Constraints: types.EngineConstraints{
			MaxTextLength: c.maxText,
			Chunking:      types.ChunkingSpec{MaxChunkSize: c.pref},
		}}
		if got := m.ChunkSize(); got != c.want {
			t.Errorf("maxText=%d pref=%d: got %d, want %d", c.maxText, c.pref, got, c.want)
		}
	}
}
