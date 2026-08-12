package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/db/sqlc"
	"backend/handlers"
	"backend/state"
	"backend/storage"

	"github.com/go-chi/chi/v5"
)

// TestLocalStore_IsNotPresigner is an invariant that the audio serving path relies on.
//
// The handler decides between redirecting and writing bytes via a type assertion to Presigner,
// not by comparing STORAGE_BACKEND to the string "s3". If LocalStore were to satisfy this
// interface, local-disk deployments would emit URLs that nothing can serve.
func TestLocalStore_IsNotPresigner(t *testing.T) {
	s, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if _, ok := any(s).(storage.Presigner); ok {
		t.Error("LocalStore must not satisfy Presigner")
	}
}

// TestS3Store_IsPresigner holds the opposite direction: if PresignGet's signature drifts from
// the interface, the handler's assertion silently returns false and every download falls back to
// passing through the backend — slower but still correct, so nothing visibly breaks to detect it.
func TestS3Store_IsPresigner(t *testing.T) {
	if _, ok := any(&storage.S3Store{}).(storage.Presigner); !ok {
		t.Error("S3Store must satisfy Presigner")
	}
}

// fakePresigner is a Store that emits URLs, so the redirect branch can be tested without a real S3.
type fakePresigner struct {
	storage.Store
	url    string
	signed []string
}

func (f *fakePresigner) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	f.signed = append(f.signed, key)
	return f.url + "?key=" + key, nil
}

// TestGetTaskAudio_RedirectsWhenStoreCanPresign locks in the reason this change exists.
//
// Before: a 563 KB file passing through the backend became 1180 KB — in once and out once —
// and sat entirely in RAM for the duration of the download. After, the same path is just a 302 response.
func TestGetTaskAudio_RedirectsWhenStoreCanPresign(t *testing.T) {
	prev := storage.Global
	defer func() { storage.Global = prev }()

	base, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	fake := &fakePresigner{Store: base, url: "https://example.invalid/obj"}
	storage.Global = fake

	const taskID = "presign-task-1"
	if err := base.Put(context.Background(), storage.AudioKey(taskID, "wav"), bytes.NewReader([]byte("RIFF....WAVE"))); err != nil {
		t.Fatalf("write object: %v", err)
	}

	task := state.GlobalTaskManager.GetOrCreate(taskID)
	task.SetOwner("owner-1")
	task.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	task.SetSourceFormat("wav")

	h := handlers.NewTasksHandler()
	r := chi.NewRouter()
	r.Get("/tasks/{task_id}/audio", h.GetTaskAudio)

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID+"/audio", nil)
	req = asUser(req, &sqlc.User{ID: "owner-1", Username: "owner", IsApproved: true})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("store that can emit URLs must return 302, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://example.invalid/obj") {
		t.Errorf("Location does not point to the store: %q", loc)
	}
	// http.Redirect writes a short HTML page with a link — standard net/http behavior, and
	// redirect-following clients never read it. What matters is that the response does NOT
	// carry audio, so we check against a size ceiling rather than demanding an empty body.
	if rec.Body.Len() > 512 {
		t.Errorf("redirect response carries %d bytes — appears to still include audio", rec.Body.Len())
	}
	if strings.Contains(rec.Body.String(), "RIFF") {
		t.Error("redirect response must not carry audio bytes")
	}
	if len(fake.signed) == 0 {
		t.Error("PresignGet should have been called")
	}
}

// TestGetTaskAudio_ChecksOwnershipBeforeSigning is the security half of the same change.
//
// A signed URL does not go through ownsTask on reuse, so the ownership check must happen BEFORE
// signing. If the order were reversed, a non-owner would still receive a usable URL — and a
// subsequent 403 would be meaningless.
func TestGetTaskAudio_ChecksOwnershipBeforeSigning(t *testing.T) {
	prev := storage.Global
	defer func() { storage.Global = prev }()

	base, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	fake := &fakePresigner{Store: base, url: "https://example.invalid/obj"}
	storage.Global = fake

	const taskID = "presign-task-2"
	if err := base.Put(context.Background(), storage.AudioKey(taskID, "wav"), bytes.NewReader([]byte("RIFF....WAVE"))); err != nil {
		t.Fatalf("write object: %v", err)
	}

	task := state.GlobalTaskManager.GetOrCreate(taskID)
	task.SetOwner("owner-2")
	task.Notify(state.TaskUpdate{Status: "done", Progress: 100})
	task.SetSourceFormat("wav")

	h := handlers.NewTasksHandler()
	r := chi.NewRouter()
	r.Get("/tasks/{task_id}/audio", h.GetTaskAudio)

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID+"/audio", nil)
	req = asUser(req, &sqlc.User{ID: "intruder-2", Username: "intruder", IsApproved: true})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("non-owner must receive 403, got %d", rec.Code)
	}
	if len(fake.signed) != 0 {
		t.Errorf("must not sign URL for non-owner, signed %v", fake.signed)
	}
}
