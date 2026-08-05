package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"core-backend/client"
	"core-backend/handlers"

	"github.com/go-chi/chi/v5"
)

func TestTTSClone_UploadVoice_Unauthenticated(t *testing.T) {
	ttsClient := client.NewCoreTTSClient("http://localhost:8001", 5)
	h := handlers.NewTTSCloneHandler(ttsClient)

	req := httptest.NewRequest(http.MethodPost, "/api/clone/upload", nil)
	rec := httptest.NewRecorder()

	h.UploadVoice(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated UploadVoice, got %d", rec.Code)
	}
}

func TestTTSClone_UploadTempVoice_Unauthenticated(t *testing.T) {
	ttsClient := client.NewCoreTTSClient("http://localhost:8001", 5)
	h := handlers.NewTTSCloneHandler(ttsClient)

	req := httptest.NewRequest(http.MethodPost, "/api/clone/upload-temp", nil)
	rec := httptest.NewRecorder()

	h.UploadTempVoice(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated UploadTempVoice, got %d", rec.Code)
	}
}

func TestTTSClone_GetUserVoices_Unauthenticated(t *testing.T) {
	ttsClient := client.NewCoreTTSClient("http://localhost:8001", 5)
	h := handlers.NewTTSCloneHandler(ttsClient)

	req := httptest.NewRequest(http.MethodGet, "/api/clone/voices", nil)
	rec := httptest.NewRecorder()

	h.GetUserVoices(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated GetUserVoices, got %d", rec.Code)
	}
}

func TestTTSClone_DeleteUserVoice_Unauthenticated(t *testing.T) {
	ttsClient := client.NewCoreTTSClient("http://localhost:8001", 5)
	h := handlers.NewTTSCloneHandler(ttsClient)

	r := chi.NewRouter()
	r.Delete("/clone/voices/{clone_id}", h.DeleteUserVoice)

	req := httptest.NewRequest(http.MethodDelete, "/clone/voices/some-clone-id", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated DeleteUserVoice, got %d", rec.Code)
	}
}
