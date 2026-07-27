package handlers

import (
	"encoding/json"
	"net/http"

	"core-backend/db"
)

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "vieneu-core-backend-go",
	})
}

func ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if db.Pool == nil || db.Pool.Ping(r.Context()) != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"detail": "Database connection error",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "ready",
		"database": "connected",
	})
}
