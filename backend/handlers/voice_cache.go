package handlers

import (
	"sync"
	"time"

	"backend/client"
	"backend/state"
)

// voiceCacheTTL is the maximum age of a preset voice fetch before it is considered stale.
//
// The engine's voice list changes rarely, so 30 seconds is longer than any voice picker
// open/close cycle while still catching up when the engine does change voices. Shorter means
// more useless HTTP calls to the engine; longer means the operator waits forever for new
// voices to appear.
const voiceCacheTTL = 30 * time.Second

// voiceCacheEntry is a preset voice fetch per mode, with timestamp and version attestation.
type voiceCacheEntry struct {
	voices          []client.CoreVoice
	manifestVersion string
	fetched         time.Time
}

// presetVoiceCache is a shared cache for all users — preset voices don't belong to any
// individual, so a shared cache is safe (unlike cloned voices, which must be scoped per user).
var presetVoiceCache = struct {
	mu   sync.Mutex
	mode map[string]*voiceCacheEntry
}{mode: make(map[string]*voiceCacheEntry)}

// voiceCacheKey normalizes an empty mode ("full view" query) to "all" to avoid keeping two
// redundant entries.
func voiceCacheKey(mode string) string {
	if mode == "" {
		return "all"
	}
	return mode
}

// getCachedPresetVoices returns cached preset voices if still fresh across TWO layers:
//
//   - manifestVersion: admin reload manifest (engine may have changed voices at that point)
//     invalidates any cache taken under the old manifest immediately — no need to wait for TTL.
//   - voiceCacheTTL: guards against the case where the manifest doesn't bump its version
//     despite voices having changed.
//
// Expired entries are deleted from the map on the spot rather than accumulating. Returns
// voices and true when a hit; false means the caller should re-fetch.
func getCachedPresetVoices(mode string, now time.Time) ([]client.CoreVoice, bool) {
	presetVoiceCache.mu.Lock()
	defer presetVoiceCache.mu.Unlock()

	e := presetVoiceCache.mode[voiceCacheKey(mode)]
	if e == nil {
		return nil, false
	}
	if e.manifestVersion != currentManifestVersion() || now.Sub(e.fetched) > voiceCacheTTL {
		delete(presetVoiceCache.mode, voiceCacheKey(mode))
		return nil, false
	}
	return e.voices, true
}

// storeCachedPresetVoices saves a preset voice fetch as cache.
func storeCachedPresetVoices(mode string, voices []client.CoreVoice, now time.Time) {
	presetVoiceCache.mu.Lock()
	presetVoiceCache.mode[voiceCacheKey(mode)] = &voiceCacheEntry{
		voices:          voices,
		manifestVersion: currentManifestVersion(),
		fetched:         now,
	}
	presetVoiceCache.mu.Unlock()
}

// currentManifestVersion returns the version of the currently active manifest; empty when
// none is loaded.
func currentManifestVersion() string {
	if m := state.GlobalManifestState.Get(); m != nil {
		return m.Version
	}
	return ""
}
