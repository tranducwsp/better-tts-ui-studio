package state

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"backend/types"
)

// EngineManifestState manages centralized RAM storage of the Universal AI Engine Manifest.
type EngineManifestState struct {
	mu       sync.RWMutex
	manifest *types.UniversalManifest
}

// GlobalManifestState is the Singleton RAM Cache state for the AI Engine Manifest.
var GlobalManifestState = &EngineManifestState{}

// Set updates the RAM Cache with a new Manifest, after consistency checks.
//
// A self-contradictory Manifest is rejected and the current one is kept: a single
// failed reload must not downgrade a healthy running Engine to a worse state than
// before the call. Milder issues are only logged — see types.Validate for the line
// between the two categories.
func (s *EngineManifestState) Set(m *types.UniversalManifest) error {
	errs, warnings := m.Validate()

	for _, w := range warnings {
		log.Printf("⚠️  Manifest: %s", w)
	}

	if len(errs) > 0 {
		for _, e := range errs {
			log.Printf("❌ Manifest rejected: %v", e)
		}
		return fmt.Errorf("invalid manifest: %w", errors.Join(errs...))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifest = m
	return nil
}

// Clear removes the held Manifest, reverting the platform to a state where no Engine
// has been detected.
//
// Separated from Set because the two are different operations: Set receives a
// declaration and must validate it, while this is an intentional return to the empty
// state. Previously the same function handled both via Set(nil), making it impossible
// to distinguish "Engine sent invalid data" from "intentionally clearing state".
func (s *EngineManifestState) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifest = nil
}

// Get retrieves the current Manifest from RAM Cache (Thread-safe).
func (s *EngineManifestState) Get() *types.UniversalManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manifest
}

// IsLoaded checks whether the Manifest has been successfully loaded from the AI Engine.
func (s *EngineManifestState) IsLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manifest != nil
}

// HasMode reports whether the Engine actually declares this Mode.
//
// ResolveAudioSpec and ResolveCapabilities both silently fall back to defaults when
// they encounter an unknown mode, so they cannot be used for validity checks. Any
// caller that takes a model_id from the client and uses it to construct a path must
// query this function first: a nonexistent mode is both an unrecoverable path and a
// place to insert "../".
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

// errManifestUnavailable is the common response when no Manifest is available to
// validate against.
//
// app.BootstrapWeb/BootstrapWorker require a Manifest before the web port or queue
// opens, so this state only occurs if someone removes it while the system is running.
// In that case, rejecting is the only correct choice: without knowing what the Engine
// accepts, there is no basis to deem a request valid.
var errManifestUnavailable = errors.New("no manifest from AI Engine yet, temporarily not accepting synthesis requests")

// ValidatePitch checks Pitch against the requested Mode, since the same Engine may
// have modes that support it and modes that do not. Pass nil when the client does not
// send Pitch.
func (s *EngineManifestState) ValidatePitch(pitch *float64, mode string) error {
	if pitch == nil {
		return nil
	}

	s.mu.RLock()
	m := s.manifest
	s.mu.RUnlock()

	// Without a Manifest we cannot know whether this Mode accepts pitch. Previously
	// this returned nil — i.e. defaulting to "supported", the more dangerous of the
	// two possible wrong guesses.
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

// ValidateEmotion checks Emotion against the Mode and cross-references the Manifest's
// SupportedEmotions. An empty string means "let the Engine decide" and is always valid.
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

// ValidateRequest performs dynamic validation based on the Manifest's Constraints and
// SupportedModes.
func (s *EngineManifestState) ValidateRequest(text string, speed float64, mode string) error {
	s.mu.RLock()
	m := s.manifest
	s.mu.RUnlock()

	if m == nil {
		return errManifestUnavailable
	}

	// The ceiling always applies: TextLimit falls back to the platform default when
	// the Engine declares 0, because "undeclared" is not the same as "unlimited".
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

	// Mode is always checked: Set() rejects a manifest that declares no modes, so
	// the list is guaranteed non-empty by this point. The old len(...) > 0 guard
	// only covered the exact case that should not be covered — an empty manifest
	// would let every mode through.
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
