package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"backend/audio"
	"backend/db"
	"backend/state"
	"backend/storage"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
)

// TasksHandler handles progress checking, Server-Sent Events (SSE) streaming, task
// cancellation, and audio file downloads.
type TasksHandler struct {
	presignTTL time.Duration
}

// NewTasksHandler creates a new TasksHandler.
func NewTasksHandler(ttl ...time.Duration) *TasksHandler {
	presignTTL := 60 * time.Second
	if len(ttl) > 0 && ttl[0] > 0 {
		presignTTL = ttl[0]
	}
	return &TasksHandler{presignTTL: presignTTL}
}

// ownsTask checks whether the caller has rights over this task, and has already written
// an error response if not.
//
// Previously the four handlers below only looked up task_id and returned the result.
// task_id is a UUID so it is hard to guess, but it is exposed in the history
// (ChunkItemResponse.TaskID) and sent by the client when assembling, so "hard to guess"
// is not access control.
//
// The owner is obtained from the in-memory TaskItem first, only querying the DB when the
// task in RAM does not carry that field — i.e. a task reconstructed from Redis after a
// restart. Status polling and SSE are the two hottest paths of a running job; a join query
// for every progress check is a cost not worth paying for information the process already
// knows.
//
// A task whose owner cannot be found in either place is rejected: better to block one
// legitimate task than to open every task to everyone because of a missing record.
func ownsTask(w http.ResponseWriter, r *http.Request, taskID string) bool {
	user, ok := currentUser(w, r)
	if !ok {
		return false
	}

	owner := ""
	if task, found := state.GlobalTaskManager.Get(taskID); found {
		if id, known := task.Owner(); known {
			owner = id
		}
	}

	if owner == "" {
		id, err := db.Queries.GetTaskOwner(r.Context(), taskID)
		if err != nil {
			writeError(w, http.StatusNotFound, "Task not found")
			return false
		}
		owner = id
	}

	if owner != user.ID && user.Role != "admin" {
		writeError(w, http.StatusForbidden, "Forbidden")
		return false
	}
	return true
}

// GetTaskStatus retrieves the status (status, progress) of an asynchronous Task by task_id.
func (h *TasksHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := chi.URLParam(r, "task_id")

	if !ownsTask(w, r, taskID) {
		return
	}

	task, ok := state.GlobalTaskManager.Get(taskID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Task not found"})
		return
	}

	status, _, progress, _ := task.Snapshot()
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"status":   status,
		"progress": progress,
	})
}

// CancelTask cancels a running or pending task.
func (h *TasksHandler) CancelTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := chi.URLParam(r, "task_id")

	if !ownsTask(w, r, taskID) {
		return
	}

	state.GlobalTaskManager.Cancel(taskID)

	// Write status to DB immediately, without waiting for the worker. Previously only Cancel
	// at the state layer existed: if the job was still queued, no one would set the chunk to
	// 'cancelled', and the tts_chunks table would stay at 'processing' forever even though the
	// user had clicked stop and the UI already showed "cancelled".
	//
	// Use context.Background() instead of r.Context(): the user may have disconnected right
	// after sending the cancel command, and this record must still take effect even if the
	// client is not listening for the response. The update is conditional (only transitions
	// from pending/processing) so it does not overwrite 'done'/'error' if the worker wins
	// the race.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.CancelChunk(ctx, taskID); err != nil {
		log.Printf("Task %s: cancelled at the state layer but could not write status to DB: %v", taskID, err)
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Cancellation requested"})
}

// GetTaskAudio returns the audio data of a Task after completion, transcoding on demand.
//
// The native format is determined by the Mode — Edge TTS returns MP3, local models return
// WAV — so the Task records what it holds. Requesting that exact format returns it directly;
// requesting a different format triggers ffmpeg transcoding on the fly and the result is
// cached so the next request skips the conversion.
func (h *TasksHandler) GetTaskAudio(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task_id")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))

	// task_id and format end up in filenames below. Do not let the query string become a
	// filepath.Join escape hatch: format=../../etc/passwd used to be concatenated into
	// <task>.to.<format> and read before CanTranscode could reject it.
	if !safeTaskID(taskID) {
		writeError(w, http.StatusBadRequest, "Invalid task ID")
		return
	}
	if !ownsTask(w, r, taskID) {
		return
	}
	if format != "" && !audio.CanTranscode(format) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Audio format '%s' is not supported", format))
		return
	}

	// Query the store before touching RAM.
	//
	// With a store that can serve URLs (S3), the fastest path is to redirect the client
	// straight there — and in that case this process does not need any bytes at all. Reading
	// TaskManager first would break exactly that: Get() backfills audio from Redis into RAM
	// when it finds RAM empty, so by the time we get here source is always non-nil and the
	// redirect branch never executes. That is why the first version of this change still
	// returned 200 with the entire file instead of 302.
	if declared := taskSourceFormat(taskID); declared != "" && format == "" {
		format = declared
	}

	if format != "" {
		key := storage.TranscodeKey(taskID, format)
		if ok, err := storage.Global.Exists(r.Context(), key); err == nil && ok {
			if h.serveFromStore(w, r, key, format) {
				return
			}
		}
		if ok, err := storage.Global.Exists(r.Context(), storage.AudioKey(taskID, format)); err == nil && ok {
			if h.serveFromStore(w, r, storage.AudioKey(taskID, format), format) {
				return
			}
		}
	}

	var source []byte
	sourceFormat := ""

	if task, ok := state.GlobalTaskManager.Get(taskID); ok {
		status, declaredFormat, _, audioBytes := task.Snapshot()
		if status == "done" {
			sourceFormat = declaredFormat
			if format == "" {
				format = sourceFormat
			}
			if len(audioBytes) > 0 {
				source = audioBytes
			}
		}
	}

	// The transcoded version from a previous download sits next to the original in the store,
	// so this time ffmpeg is not needed. The cleanup job reclaims both under the same policy.
	if format != "" {
		if b, err := storage.Global.Get(r.Context(), storage.TranscodeKey(taskID, format)); err == nil && len(b) > 0 {
			writeAudio(w, b, format)
			return
		}
	}

	// The task may have been evicted from RAM; the store object carries the native format
	// as its extension.
	//
	// Probe with Exists before loading: when the client asks for the native format — the
	// most common case — there is no need to load bytes into this process at all.
	if source == nil {
		for _, ext := range audio.KnownFormats() {
			key := storage.AudioKey(taskID, ext)
			ok, err := storage.Global.Exists(r.Context(), key)
			if err != nil || !ok {
				continue
			}
			if format == "" {
				format = ext
			}
			if format == ext && h.serveFromStore(w, r, key, ext) {
				return
			}
			b, err := storage.Global.Get(r.Context(), key)
			if err != nil || len(b) == 0 {
				continue
			}
			source, sourceFormat = b, ext
			break
		}
	}

	if source == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Audio is not ready"})
		return
	}

	if format == "" || format == sourceFormat {
		writeAudio(w, source, sourceFormat)
		return
	}

	converted, err := audio.Transcode(r.Context(), source, format)
	if err != nil {
		log.Printf("Transcode task %s from %s to %s failed: %v", taskID, sourceFormat, format, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"detail": fmt.Sprintf("Could not convert audio to %s", format),
		})
		return
	}

	state.GlobalTaskManager.CacheTranscoded(taskID, format, converted)
	// The store copy is only a cache for the next download; a failed write only means the next
	// time runs ffmpeg again, so there is no need to spoil a successful response. Still log so
	// a full disk does not manifest as "why is downloading slow lately".
	transKey := storage.TranscodeKey(taskID, format)
	if err := storage.Global.Put(r.Context(), transKey, bytes.NewReader(converted)); err != nil {
		log.Printf("Could not store transcoded version %s.%s: %v", taskID, format, err)
		// Cannot store, so cannot sign: the URL would point to a non-existent object.
		writeAudio(w, converted, format)
		return
	}
	if h.serveFromStore(w, r, transKey, format) {
		return
	}
	writeAudio(w, converted, format)
}

// taskSourceFormat reads the format the Engine produced, without pulling in bytes.
//
// Separated from TaskManager.Get because that function backfills audio from Redis into RAM
// as a side effect — useful for the bytes-serving path, but exactly what we want to avoid here.
func taskSourceFormat(taskID string) string {
	task, ok := state.GlobalTaskManager.Peek(taskID)
	if !ok {
		return ""
	}
	status, format, _, _ := task.Snapshot()
	if status != "done" {
		return ""
	}
	return format
}

// presignTTL is the default lifetime of a direct download URL when the handler is constructed
// outside of bootstrap. Production passes the value from Config; the default only keeps old
// tests/consumers running safely.
const defaultPresignTTL = 60 * time.Second

// serveFromStore serves audio to the client and reports whether it was able to do so.
//
// With a store that can serve URLs (S3), sends a 302 to a signed URL: the client downloads
// directly from the store, so the backend is no longer a pipe. Measured before the change:
// a 563 KB file going through the backend became 1180 KB — in once and out once — and sat
// entirely in RAM for the duration of the download.
//
// With a store that cannot serve URLs (local disk), returns false so the caller uses the
// old path: read bytes then write to the response.
func (h *TasksHandler) serveFromStore(w http.ResponseWriter, r *http.Request, key, format string) bool {
	ps, ok := storage.Global.(storage.Presigner)
	if !ok {
		return false
	}

	url, err := ps.PresignGet(r.Context(), key, h.presignTTL)
	if err != nil {
		// A failed signature is not a reason to deny the user: the bytes-reading path is
		// still available.
		log.Printf("Could not sign URL for %s: %v — falling back to backend streaming", key, err)
		return false
	}

	// 302, not 301: this URL expires after presignTTL, so it must not be cached as a
	// permanent location.
	w.Header().Set("Content-Disposition", `attachment; filename="tts_studio_audio.`+format+`"`)
	http.Redirect(w, r, url, http.StatusFound)
	return true
}

// safeTaskID only allows characters that the platform's UUID/task IDs use.
// Does not use filepath.Base to "sanitize": sanitizing a malicious path can still point to
// a file outside the directory if the remainder is further concatenated.
func safeTaskID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// writeAudio writes audio data with the correct Content-Type and filename.
func writeAudio(w http.ResponseWriter, data []byte, format string) {
	w.Header().Set("Content-Type", audio.MimeType(format))
	w.Header().Set("Content-Disposition", `attachment; filename="tts_studio_audio.`+format+`"`)
	_, _ = w.Write(data)
}

// StreamTaskProgress streams real-time progress data via an HTTP Persistent/Event-Stream
// (SSE) connection.
func (h *TasksHandler) StreamTaskProgress(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task_id")

	if !ownsTask(w, r, taskID) {
		return
	}

	task, ok := state.GlobalTaskManager.Get(taskID)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Task not found"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Lift the write deadline for this stream alone.
	//
	// The server sets WriteTimeout to 120 seconds so a client that holds a connection open
	// cannot hold resources forever. But that deadline applies to the entire response, and
	// the response here lasts exactly as long as the synthesis job — so every job longer than
	// two minutes gets cut off mid-stream by net/http, precisely the kind of job SSE was
	// created to serve. The browser sees the stream break and reconnects, creating yet another
	// stream that will also be cut.
	//
	// Removing the deadline here does not lose the protection layer: the loop below exits
	// immediately when r.Context() is cancelled, i.e. when the client disconnects.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("SSE task %s: could not lift write deadline (%v); stream will stop when WriteTimeout is reached", taskID, err)
	}

	ch := task.Subscribe()
	defer task.Unsubscribe(ch)

	// Send the initial event
	status, _, progress, _ := task.Snapshot()
	initUpdate := state.TaskUpdate{
		Status:   status,
		Progress: progress,
		Error:    task.LastError(),
	}
	initBytes, _ := sonic.Marshal(initUpdate)
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(initBytes)
	_, _ = w.Write([]byte("\n\n"))
	flusher.Flush()

	if status == "done" || status == "error" || status == "cancelled" {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case update, open := <-ch:
			if !open {
				return
			}
			updateBytes, _ := sonic.Marshal(update)
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(updateBytes)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()

			if update.Status == "done" || update.Status == "error" || update.Status == "cancelled" {
				return
			}
		}
	}
}
