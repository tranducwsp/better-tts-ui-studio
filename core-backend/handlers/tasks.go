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
// Định dạng gốc do Mode quyết định — Edge TTS trả MP3, mô hình cục bộ trả WAV — nên Task
// ghi lại mình đang giữ gì. Hỏi đúng định dạng đó thì trả thẳng; hỏi khác thì ffmpeg
// chuyển mã tại chỗ và kết quả được ghi nhớ để lần sau khỏi chạy lại.
func (h *TasksHandler) GetTaskAudio(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task_id")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))

	var source []byte
	sourceFormat := ""

	if task, ok := state.GlobalTaskManager.Get(taskID); ok && task.Status == "done" {
		sourceFormat = task.SourceFormat
		if format == "" {
			format = sourceFormat
		}
		// Đã có sẵn bản đúng định dạng thì khỏi làm gì thêm.
		if b, cached := task.Transcoded[format]; cached && len(b) > 0 {
			writeAudio(w, b, format)
			return
		}
		if len(task.Audio) > 0 {
			source = task.Audio
		}
	}

	// Task có thể đã bị dọn khỏi RAM; tệp trên đĩa mang phần mở rộng là định dạng gốc.
	if source == nil {
		for _, ext := range audio.KnownFormats() {
			b, err := os.ReadFile(filepath.Join(storage.TempDir(), taskID+"."+ext))
			if err != nil || len(b) == 0 {
				continue
			}
			source, sourceFormat = b, ext
			if format == "" {
				format = ext
			}
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
		log.Printf("Transcode task %s from %s to %s failed: %v", taskID, sourceFormat, format, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
			"detail": fmt.Sprintf("Could not convert audio to %s", format),
		})
		return
	}

	state.GlobalTaskManager.CacheTranscoded(taskID, format, converted)
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
