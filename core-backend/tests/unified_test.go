package tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"core-backend/client"
	"core-backend/handlers"

	"github.com/go-chi/chi/v5"
)

func TestUnified_GetVoices_Unauthenticated(t *testing.T) {
	ttsClient := client.NewCoreTTSClient("http://localhost:8001", 5)
	h := handlers.NewUnifiedHandler(ttsClient)

	r := chi.NewRouter()
	r.Get("/voices/{model_id}", h.GetVoices)

	req := httptest.NewRequest(http.MethodGet, "/voices/standard", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated GetVoices, got %d", rec.Code)
	}
}

func TestUnified_Synthesize_Unauthenticated(t *testing.T) {
	ttsClient := client.NewCoreTTSClient("http://localhost:8001", 5)
	h := handlers.NewUnifiedHandler(ttsClient)

	r := chi.NewRouter()
	r.Post("/synthesize/{model_id}", h.Synthesize)

	req := httptest.NewRequest(http.MethodPost, "/synthesize/standard", bytes.NewReader([]byte(`{"text":"Hello world"}`)))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated Synthesize, got %d", rec.Code)
	}
}
