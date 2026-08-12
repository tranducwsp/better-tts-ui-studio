package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/db/sqlc"
	"backend/handlers"
	"backend/middleware"
	"backend/state"

	"github.com/go-chi/chi/v5"
)

// taskOwner is the user that all tasks in this file belong to.
//
// The task handler now requires both identity and ownership, so every test request must carry
// a user context — otherwise everything stops at 401 and we are only testing the auth layer.
var taskOwner = &sqlc.User{ID: "task-owner-1", Username: "owner", IsApproved: true}

// asUser attaches a user to the request just like AuthMiddleware does at runtime.
func asUser(req *http.Request, user *sqlc.User) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, user))
}

// ownedTask creates a task with an owner recorded, so ownsTask does not need to query the DB (tests have no DB).
func ownedTask(taskID string) *state.TaskItem {
	task := state.GlobalTaskManager.GetOrCreate(taskID)
	task.SetOwner(taskOwner.ID)
	return task
}

func TestGetTaskStatus_Unauthenticated(t *testing.T) {
	h := handlers.NewTasksHandler()
	r := chi.NewRouter()
	r.Get("/tasks/{task_id}", h.GetTaskStatus)

	req := httptest.NewRequest(http.MethodGet, "/tasks/test-task-1", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for unauthenticated task status, got %d", rec.Code)
	}
}

// TestGetTaskStatus_OtherUsersTask locks in the very vulnerability that was patched: a task
// with an owner must not be returned to someone else, even if they know the task_id.
func TestGetTaskStatus_OtherUsersTask(t *testing.T) {
	h := handlers.NewTasksHandler()
	ownedTask("someone-elses-task")

	r := chi.NewRouter()
	r.Get("/tasks/{task_id}", h.GetTaskStatus)

	intruder := &sqlc.User{ID: "intruder-9", Username: "intruder", IsApproved: true}
	req := asUser(httptest.NewRequest(http.MethodGet, "/tasks/someone-elses-task", nil), intruder)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 when reading another user's task, got %d", rec.Code)
	}
}

// TestCancelTask_OtherUsersTask confirms an outsider cannot cancel another user's running
// job — a write, so this has heavier consequences than merely reading status.
func TestCancelTask_OtherUsersTask(t *testing.T) {
	h := handlers.NewTasksHandler()
	task := ownedTask("victim-task")

	r := chi.NewRouter()
	r.Post("/tasks/{task_id}/cancel", h.CancelTask)

	intruder := &sqlc.User{ID: "intruder-9", Username: "intruder", IsApproved: true}
	req := asUser(httptest.NewRequest(http.MethodPost, "/tasks/victim-task/cancel", nil), intruder)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 when cancelling another user's task, got %d", rec.Code)
	}
	if task.IsCancelled() {
		t.Errorf("Task of another user must not be cancelled")
	}
}

// TestGetTaskAudio_OtherUsersTask confirms audio does not leak to other users.
func TestGetTaskAudio_OtherUsersTask(t *testing.T) {
	h := handlers.NewTasksHandler()
	task := ownedTask("private-audio-task")
	task.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	task.SetSourceFormat("wav")
	task.SetAudio([]byte("RIFF mock wav audio bytes"))

	r := chi.NewRouter()
	r.Get("/tasks/{task_id}/audio", h.GetTaskAudio)

	intruder := &sqlc.User{ID: "intruder-9", Username: "intruder", IsApproved: true}
	req := asUser(httptest.NewRequest(http.MethodGet, "/tasks/private-audio-task/audio", nil), intruder)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 when downloading another user's audio, got %d", rec.Code)
	}
}

func TestGetTaskStatus_Success(t *testing.T) {
	h := handlers.NewTasksHandler()

	task := ownedTask("test-task-1")
	task.Notify(state.TaskUpdate{Status: "processing", Progress: 45})

	r := chi.NewRouter()
	r.Get("/tasks/{task_id}", h.GetTaskStatus)

	req := asUser(httptest.NewRequest(http.MethodGet, "/tasks/test-task-1", nil), taskOwner)
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

	task := ownedTask("audio-task-wav")
	task.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	task.SetSourceFormat("wav")
	task.SetAudio([]byte("RIFF mock wav audio bytes"))

	r := chi.NewRouter()
	r.Get("/tasks/{task_id}/audio", h.GetTaskAudio)

	req := asUser(httptest.NewRequest(http.MethodGet, "/tasks/audio-task-wav/audio", nil), taskOwner)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("Content-Type") != "audio/wav" {
		t.Errorf("Expected Content-Type audio/wav, got %s", rec.Header().Get("Content-Type"))
	}
}

// TestGetTaskAudio_RejectsTraversalTaskID keeps the guard on the URL path: the task ID goes
// into a filename, so it must be rejected before anyone reads from disk.
func TestGetTaskAudio_RejectsTraversalTaskID(t *testing.T) {
	h := handlers.NewTasksHandler()

	r := chi.NewRouter()
	r.Get("/tasks/{task_id}/audio", h.GetTaskAudio)

	req := asUser(httptest.NewRequest(http.MethodGet, "/tasks/..%2f..%2fetc%2fpasswd/audio", nil), taskOwner)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for traversal task ID, got %d", rec.Code)
	}
}

func TestCancelTask(t *testing.T) {
	h := handlers.NewTasksHandler()

	task := ownedTask("cancel-task-1")
	task.Notify(state.TaskUpdate{Status: "processing", Progress: 0})

	r := chi.NewRouter()
	r.Post("/tasks/{task_id}/cancel", h.CancelTask)

	req := asUser(httptest.NewRequest(http.MethodPost, "/tasks/cancel-task-1/cancel", nil), taskOwner)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if !task.IsCancelled() {
		t.Errorf("Task cancel flag should be true")
	}
}

func TestStreamTaskProgress(t *testing.T) {
	h := handlers.NewTasksHandler()

	task := ownedTask("stream-task-1")
	task.Notify(state.TaskUpdate{Status: "done", Progress: 100})

	r := chi.NewRouter()
	r.Get("/stream/tasks/{task_id}", h.StreamTaskProgress)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/stream/tasks/stream-task-1", nil).WithContext(ctx)
	req = asUser(req, taskOwner)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected event-stream content type, got %s", rec.Header().Get("Content-Type"))
	}
}

// TestTaskOwner_NotStolenBySecondCaller: task_id is sent by the client, so a second call
// with the same task_id must not steal the first caller's ownership.
func TestTaskOwner_NotStolenBySecondCaller(t *testing.T) {
	task := ownedTask("ownership-fixed")
	task.SetOwner("intruder-9")

	owner, known := task.Owner()
	if !known || owner != taskOwner.ID {
		t.Errorf("Expected owner to stay %s, got %q (known=%v)", taskOwner.ID, owner, known)
	}
}
