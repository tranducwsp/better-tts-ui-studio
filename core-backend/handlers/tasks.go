package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"core-backend/state"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
)

type TasksHandler struct{}

func NewTasksHandler() *TasksHandler {
	return &TasksHandler{}
}

func (h *TasksHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := chi.URLParam(r, "task_id")

	task, ok := state.GlobalTaskManager.Get(taskID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Không tìm thấy task"})
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]interface{}{
		"status":   task.Status,
		"progress": task.Progress,
	})
}

func (h *TasksHandler) CancelTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := chi.URLParam(r, "task_id")

	state.GlobalTaskManager.Cancel(taskID)
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Đã yêu cầu hủy"})
}

func (h *TasksHandler) GetTaskAudio(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task_id")
	format := strings.ToLower(r.URL.Query().Get("format"))

	task, ok := state.GlobalTaskManager.Get(taskID)
	if !ok || task.Status != "done" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Audio chưa sẵn sàng"})
		return
	}

	if format == "mp3" && len(task.AudioMP3) > 0 {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Disposition", `attachment; filename="vieneu_tts_audio.mp3"`)
		_, _ = w.Write(task.AudioMP3)
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Disposition", `attachment; filename="vieneu_tts_audio.wav"`)
	_, _ = w.Write(task.AudioWAV)
}

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

	// Send current initial status
	initUpdate := state.TaskUpdate{
		Status:   task.Status,
		Progress: task.Progress,
		Error:    task.Error,
	}
	initBytes, _ := sonic.Marshal(initUpdate)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(initBytes))
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
			_, _ = fmt.Fprintf(w, "data: %s\n\n", string(updateBytes))
			flusher.Flush()

			if update.Status == "done" || update.Status == "error" || update.Status == "cancelled" {
				return
			}
		}
	}
}
