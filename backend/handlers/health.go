package handlers

import (
	"net/http"

	"backend/db"
	"backend/state"

	"github.com/bytedance/sonic"
)

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "ai-backend-go",
	})
}

func ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if db.Pool == nil || db.Pool.Ping(r.Context()) != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": "Database connection error",
		})
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"status":   "ready",
		"database": "connected",
	})
}

// GetEngineInfo serves GET /api/info, fetching the Manifest directly from the ultra-fast RAM Cache (<1ms)
func GetEngineInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	manifest := state.GlobalManifestState.Get()
	if manifest == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"error": "AI Engine Manifest has not been loaded successfully",
		})
		return
	}
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(manifest)
}
