package handlers

import (
	"backend/internal/presetvoicecache"
	"backend/state"
)

// presetVoiceCache là cache chung cho mọi người dùng. Handler sở hữu wiring tới manifest global;
// component cache chỉ biết một callback version và không phụ thuộc vào state của ứng dụng.
var presetVoiceCache = presetvoicecache.New(presetvoicecache.DefaultTTL, currentManifestVersion)

func currentManifestVersion() string {
	if m := state.GlobalManifestState.Get(); m != nil {
		return m.Version
	}
	return ""
}
