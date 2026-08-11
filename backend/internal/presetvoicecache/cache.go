package presetvoicecache

import (
	"sync"
	"time"

	"backend/client"
)

// DefaultTTL is the maximum age of a preset voice fetch before it is considered stale.
//
// The engine's voice list changes rarely, so 30 seconds avoids repeated HTTP calls when the user
// opens the voice picker, while still catching up if the engine changes voices without bumping
// the manifest version.
const DefaultTTL = 30 * time.Second

type entry struct {
	voices          []client.CoreVoice
	manifestVersion string
	fetched         time.Time
}

// Cache holds a shared per-mode preset voice list. Preset voices are not user-specific;
// cloned voices do not go through this component.
type Cache struct {
	mu              sync.Mutex
	mode            map[string]*entry
	ttl             time.Duration
	manifestVersion func() string
}

// New creates a cache with the given TTL and manifest version source. The callback keeps this
// package independent of the handler's global state and lets each deployment decide where the
// manifest lives.
func New(ttl time.Duration, manifestVersion func() string) *Cache {
	if manifestVersion == nil {
		manifestVersion = func() string { return "" }
	}
	return &Cache{
		mode:            make(map[string]*entry),
		ttl:             ttl,
		manifestVersion: manifestVersion,
	}
}

// Get returns a cached entry when both the TTL and manifest version are still valid. Expired
// entries are removed from the map immediately to avoid accumulating stale data.
func (c *Cache) Get(mode string, now time.Time) ([]client.CoreVoice, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(mode)
	e := c.mode[key]
	if e == nil {
		return nil, false
	}
	if e.manifestVersion != c.manifestVersion() || now.Sub(e.fetched) > c.ttl {
		delete(c.mode, key)
		return nil, false
	}
	return e.voices, true
}

// Store saves a preset voice fetch with the manifest version at the time it was fetched from the engine.
func (c *Cache) Store(mode string, voices []client.CoreVoice, now time.Time) {
	c.mu.Lock()
	c.mode[cacheKey(mode)] = &entry{
		voices:          voices,
		manifestVersion: c.manifestVersion(),
		fetched:         now,
	}
	c.mu.Unlock()
}

func cacheKey(mode string) string {
	if mode == "" {
		return "all"
	}
	return mode
}
