package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/handlers"
	"backend/state"
	"backend/types"
)

func TestHealthCheck(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handlers.HealthCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Failed to parse response body: %v", err)
	}

	if body["status"] != "ok" || body["service"] != "ai-backend-go" {
		t.Errorf("Unexpected body content: %v", body)
	}
}

func TestReadinessCheck_DatabaseNil(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handlers.ReadinessCheck(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d when DB is nil, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestGetEngineInfo_NotLoaded(t *testing.T) {
	state.GlobalManifestState.Clear()

	req := httptest.NewRequest(http.MethodGet, "/api/info", nil)
	rec := httptest.NewRecorder()

	handlers.GetEngineInfo(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d when manifest is nil, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestGetEngineInfo_LoadedSuccess(t *testing.T) {
	mockManifest := &types.UniversalManifest{
		EngineID:       "unit-test-engine",
		EngineName:     "Unit Test Engine",
		Version:        "1.0.0",
		Provider:       "Test Provider",
		SupportedModes: []types.EngineModeSpec{{ID: "standard", Name: "Standard"}},
	}
	if err := state.GlobalManifestState.Set(mockManifest); err != nil {
		t.Fatalf("valid manifest was rejected: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/info", nil)
	rec := httptest.NewRecorder()

	handlers.GetEngineInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var res types.UniversalManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if res.EngineID != "unit-test-engine" {
		t.Errorf("Expected EngineID 'unit-test-engine', got '%s'", res.EngineID)
	}
}
