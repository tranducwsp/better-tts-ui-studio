package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"core-backend/handlers"
	"core-backend/state"

	"github.com/go-chi/chi/v5"
)

func TestGetTaskStatus_NotFound(t *testing.T) {
	h := handlers.NewTasksHandler()
	r := chi.NewRouter()
	r.Get("/tasks/{task_id}", h.GetTaskStatus)

	req := httptest.NewRequest(http.MethodGet, "/tasks/non-existent-task-id", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestGetTaskStatus_Success(t *testing.T) {
	h := handlers.NewTasksHandler()

	// Prepare task in state
	task := state.GlobalTaskManager.GetOrCreate("test-task-1")
	task.Status = "processing"
	task.Progress = 45

	r := chi.NewRouter()
	r.Get("/tasks/{task_id}", h.GetTaskStatus)

	req := httptest.NewRequest(http.MethodGet, "/tasks/test-task-1", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body["status"] != "processing" || float64(body["progress"].(float64)) != 45 {
		t.Errorf("Unexpected body content: %v", body)
	}
}

func TestGetTaskAudio_SuccessWAV(t *testing.T) {
	h := handlers.NewTasksHandler()

	task := state.GlobalTaskManager.GetOrCreate("audio-task-wav")
	task.Status = "done"
	task.Audio = []byte("RIFF mock wav audio bytes")
	task.SourceFormat = "wav"

	r := chi.NewRouter()
	r.Get("/tasks/{task_id}/audio", h.GetTaskAudio)

	req := httptest.NewRequest(http.MethodGet, "/tasks/audio-task-wav/audio", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("Content-Type") != "audio/wav" {
		t.Errorf("Expected Content-Type audio/wav, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestCancelTask(t *testing.T) {
	h := handlers.NewTasksHandler()

	task := state.GlobalTaskManager.GetOrCreate("cancel-task-1")
	task.Status = "processing"

	r := chi.NewRouter()
	r.Post("/tasks/{task_id}/cancel", h.CancelTask)

	req := httptest.NewRequest(http.MethodPost, "/tasks/cancel-task-1/cancel", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if !task.Cancel {
		t.Errorf("Task cancel flag should be true")
	}
}

func TestStreamTaskProgress(t *testing.T) {
	h := handlers.NewTasksHandler()

	task := state.GlobalTaskManager.GetOrCreate("stream-task-1")
	task.Status = "done"
	task.Progress = 100

	r := chi.NewRouter()
	r.Get("/stream/tasks/{task_id}", h.StreamTaskProgress)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/stream/tasks/stream-task-1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected event-stream content type, got %s", rec.Header().Get("Content-Type"))
	}
}
