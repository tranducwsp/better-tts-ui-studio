package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"core-backend/audio"
	"core-backend/state"
	"core-backend/storage"

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

// GetTaskAudio trả về dữ liệu âm thanh của Task sau khi hoàn tất, chuyển mã theo yêu cầu.
//
// Nguồn âm thanh gốc là WAV do Engine sinh ra. Nếu client hỏi định dạng khác, ffmpeg sẽ
// chuyển mã tại thời điểm gọi và kết quả được ghi nhớ (cache) trong Task để lần sau không
// phải chuyển lại.
func (h *TasksHandler) GetTaskAudio(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task_id")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "wav"
	}

	// 1. Lấy dữ liệu WAV gốc, từ RAM hoặc từ đĩa.
	var source []byte
	if task, ok := state.GlobalTaskManager.Get(taskID); ok && task.Status == "done" {
		if format == "mp3" && len(task.AudioMP3) > 0 {
			writeAudio(w, task.AudioMP3, "mp3")
			return
		}
		if len(task.AudioWAV) > 0 {
			source = task.AudioWAV
		}
	}

	if source == nil {
		for _, ext := range []string{"wav", "mp3"} {
			if b, err := os.ReadFile(filepath.Join(storage.TempDir(), taskID+"."+ext)); err == nil && len(b) > 0 {
				// Tập tin trên đĩa đã đúng định dạng được hỏi thì trả về trực tiếp.
				if ext == format {
					writeAudio(w, b, format)
					return
				}
				source = b
				break
			}
		}
	}

	if source == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Audio is not ready"})
		return
	}

	// 2. Định dạng gốc là WAV, nên hỏi WAV thì trả luôn.
	if format == "wav" {
		writeAudio(w, source, "wav")
		return
	}

	if !audio.CanTranscode(format) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"detail": fmt.Sprintf("Audio format '%s' is not supported", format),
		})
		return
	}

	converted, err := audio.Transcode(r.Context(), source, format)
	if err != nil {
		log.Printf("Transcode task %s to %s failed: %v", taskID, format, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"detail": fmt.Sprintf("Could not convert audio to %s", format),
		})
		return
	}

	// 3. Ghi nhớ MP3 vì đây là định dạng được tải nhiều nhất.
	if format == "mp3" {
		state.GlobalTaskManager.SetAudioMP3(taskID, converted)
	}

	writeAudio(w, converted, format)
}

// writeAudio ghi dữ liệu âm thanh kèm Content-Type và tên tập tin đúng định dạng.
func writeAudio(w http.ResponseWriter, data []byte, format string) {
	w.Header().Set("Content-Type", audio.MimeType(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="tts_studio_audio.%s"`, format))
	_, _ = w.Write(data)
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
