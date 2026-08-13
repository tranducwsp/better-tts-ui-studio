package handlers

import (
	"io"
	"log"
	"net/http"
	"time"

	"backend/client"
	"backend/state"

	"github.com/bytedance/sonic"
)

// rebuildSignalTimeout limits the webhook call to the frontend-builder.
//
// The builder only needs to RECEIVE the signal; it replies 202 immediately and builds the bundle
// in the background, so this call never needs to wait for a build. A few seconds is more than
// enough to know the signal has arrived.
const rebuildSignalTimeout = 10 * time.Second

// rebuildClient is separated from http.DefaultClient because DefaultClient has NO timeout.
//
// Previously the webhook used http.Post, meaning DefaultClient: if the builder's port was open
// but not responding — exactly the state it falls into while busy building — the goroutine below
// waited indefinitely. Each manifest reload added one permanently stuck goroutine, and nothing
// in the logs indicated they were accumulating.
var rebuildClient = &http.Client{Timeout: rebuildSignalTimeout}

// EngineSyncHandler handles configuration/manifest sync requests from the internal AI Engine.
type EngineSyncHandler struct {
	TTSClient    *client.CoreTTSClient
	FEBuilderURL string
}

// NewEngineSyncHandler initializes EngineSyncHandler with CoreTTSClient and FEBuilderURL.
func NewEngineSyncHandler(ttsClient *client.CoreTTSClient, feBuilderURL string) *EngineSyncHandler {
	return &EngineSyncHandler{
		TTSClient:    ttsClient,
		FEBuilderURL: feBuilderURL,
	}
}

// ReloadManifest handles POST /api/internal/engine/reload requests.
// Automatically re-queries the latest Manifest from the Core AI Engine and triggers a static HTML re-build on the Frontend Builder.
func (h *EngineSyncHandler) ReloadManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	manifest, err := h.TTSClient.GetInfo()
	if err != nil {
		// This route is admin-only and its purpose is diagnostic, so the real cause is still
		// returned — but it must also go to the log, since the person pressing reload is not
		// necessarily the person reading the log when the issue occurs.
		log.Printf("Failed to fetch manifest from Engine: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": "Cannot connect to Core AI Engine to sync Manifest: " + err.Error(),
		})
		return
	}

	if manifest == nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": "Manifest received from Core AI Engine is empty",
		})
		return
	}

	// Update the internal RAM Cache. A conflicting Manifest is rejected and the current one is
	// preserved, so a failed reload does not leave the system in a worse state than before the
	// call.
	if err := state.GlobalManifestState.Set(manifest); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": "Manifest from Core AI Engine is invalid, kept the current version: " + err.Error(),
		})
		return
	}
	log.Printf("Successfully re-synced AI Engine Manifest: %s (v%s) [Max Text: %d]",
		manifest.EngineName, manifest.Version, manifest.Constraints.MaxTextLength)

	// Fire an async signal to the frontend-builder container to re-prerender static HTML
	if h.FEBuilderURL != "" {
		go func(url string) {
			rebuildTarget := url + "/rebuild"

			req, err := http.NewRequest(http.MethodPost, rebuildTarget, nil)
			if err != nil {
				log.Printf("Failed to build request to FE Builder (%s): %v", rebuildTarget, err)
				return
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := rebuildClient.Do(req)
			if err != nil {
				log.Printf("Trigger signal to FE Builder failed (%s): %v", rebuildTarget, err)
				return
			}
			// Drain the body before closing so the connection can be reused instead of discarded.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			log.Printf("Successfully triggered static HTML re-build signal to FE Builder (%s)", rebuildTarget)
		}(h.FEBuilderURL)
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"message":     "Engine manifest synchronized successfully",
		"engine_name": manifest.EngineName,
		"version":     manifest.Version,
	})
}
