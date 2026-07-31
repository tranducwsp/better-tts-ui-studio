package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"core-backend/client"
	"core-backend/handlers"
)

func TestEngineSync_ReloadManifest_Failure(t *testing.T) {
	invalidClient := client.NewCoreTTSClient("http://127.0.0.1:59999", 1)
	engineSyncHandler := handlers.NewEngineSyncHandler(invalidClient, "")

	req := httptest.NewRequest(http.MethodPost, "/api/internal/engine/reload", nil)
	rr := httptest.NewRecorder()

	engineSyncHandler.ReloadManifest(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("Expected status BadGateway (502), got %d", rr.Code)
	}
}
