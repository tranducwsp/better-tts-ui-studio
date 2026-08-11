package handlers

import (
	"backend/internal/presetvoicecache"
	"backend/state"
)

// presetVoiceCache is a shared cache for all users. The handler wires it to the global manifest;
// the cache component only knows a version callback and has no dependency on application state.
var presetVoiceCache = presetvoicecache.New(presetvoicecache.DefaultTTL, currentManifestVersion)

func currentManifestVersion() string {
	if m := state.GlobalManifestState.Get(); m != nil {
		return m.Version
	}
	return ""
}
