package handlers

import (
	"net/http"

	"core-backend/db"

	"github.com/bytedance/sonic"
)

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "vieneu-core-backend-go",
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
