package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"core-backend/state"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
)

// TasksHandler xử lý việc kiểm tra tiến độ, stream Server-Sent Events (SSE), hủy task và tải file audio.
type TasksHandler struct{}

// NewTasksHandler khởi tạo TasksHandler.
func NewTasksHandler() *TasksHandler {
	return &TasksHandler{}
}

// GetTaskStatus lấy trạng thái (status, progress) của một Task bất đồng bộ qua task_id.
func (h *TasksHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := chi.URLParam(r, "task_id")

	task, ok := state.GlobalTaskManager.Get(taskID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Task not found"})
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"status":   task.Status,
		"progress": task.Progress,
	})
}

// CancelTask cancels a running or pending task.
func (h *TasksHandler) CancelTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := chi.URLParam(r, "task_id")

	state.GlobalTaskManager.Cancel(taskID)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Cancellation requested"})
}

// GetTaskAudio gets audio bytes (WAV or MP3) after task completes.
func (h *TasksHandler) GetTaskAudio(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task_id")
	format := strings.ToLower(r.URL.Query().Get("format"))

	task, ok := state.GlobalTaskManager.Get(taskID)
	if ok && task.Status == "done" {
		if format == "mp3" && len(task.AudioMP3) > 0 {
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("Content-Disposition", `attachment; filename="tts_studio_audio.mp3"`)
			_, _ = w.Write(task.AudioMP3)
			return
		}
		if len(task.AudioWAV) > 0 {
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("Content-Disposition", `attachment; filename="tts_studio_audio.wav"`)
			_, _ = w.Write(task.AudioWAV)
			return
		}
	}

	// Fallback 1: Read file from disk storage/temp/{taskID}.wav
	filePathWAV := filepath.Join("storage/temp", taskID+".wav")
	if wavBytes, err := os.ReadFile(filePathWAV); err == nil && len(wavBytes) > 0 {
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Content-Disposition", `attachment; filename="tts_studio_audio.wav"`)
		_, _ = w.Write(wavBytes)
		return
	}

	// Fallback 2: Read file from disk storage/temp/{taskID}.mp3
	filePathMP3 := filepath.Join("storage/temp", taskID+".mp3")
	if mp3Bytes, err := os.ReadFile(filePathMP3); err == nil && len(mp3Bytes) > 0 {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Disposition", `attachment; filename="tts_studio_audio.mp3"`)
		_, _ = w.Write(mp3Bytes)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Audio is not ready"})
}

// StreamTaskProgress truyền dữ liệu tiến độ thời gian thực (Real-time SSE Stream) qua kết nối HTTP Persistent/Event-Stream.
func (h *TasksHandler) StreamTaskProgress(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task_id")

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

	ch := task.Subscribe()
	defer task.Unsubscribe(ch)

	// Gửi sự kiện khởi tạo ban đầu
	initUpdate := state.TaskUpdate{
		Status:   task.Status,
		Progress: task.Progress,
		Error:    task.Error,
	}
	initBytes, _ := sonic.Marshal(initUpdate)
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(initBytes)
	_, _ = w.Write([]byte("\n\n"))
	flusher.Flush()

	if task.Status == "done" || task.Status == "error" || task.Status == "cancelled" {
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
