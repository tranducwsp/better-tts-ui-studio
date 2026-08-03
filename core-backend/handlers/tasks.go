package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// task_id và format đi vào tên file bên dưới. Không để query string trở thành một
	// filepath.Join escape hatch: format=../../etc/passwd trước đây được ghép vào
	// <task>.to.<format> rồi đọc trước khi CanTranscode kịp từ chối.
	if !safeTaskID(taskID) {
		writeError(w, http.StatusBadRequest, "Invalid task ID")
		return
	}
	if format != "" && !audio.CanTranscode(format) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Audio format '%s' is not supported", format))
		return
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

	// Bản đã chuyển mã của lần tải trước nằm trên đĩa cạnh bản gốc, nên lần này khỏi gọi
	// ffmpeg. Bộ quét dọn thu hồi cả hai theo cùng một chính sách.
	if format != "" {
		if b, err := os.ReadFile(transcodePath(taskID, format)); err == nil && len(b) > 0 {
			writeAudio(w, b, format)
			return
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
	_ = os.WriteFile(transcodePath(taskID, format), converted, 0644)
	writeAudio(w, converted, format)
}

// safeTaskID chỉ cho phép các ký tự mà UUID/task ID của nền tảng sử dụng.
// Không dùng filepath.Base để "làm sạch": làm sạch một path độc vẫn có thể trỏ tới tệp
// ngoài thư mục nếu phần còn lại được ghép tiếp.
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

// transcodePath là nơi cất bản đã chuyển mã, cạnh bản gốc trong thư mục tạm.
//
// Hậu tố tách bằng dấu chấm để bộ quét dọn nhìn thấy chúng như mọi tệp tạm khác, và để
// vòng dò định dạng gốc ở trên không nhầm một bản chuyển mã là bản gốc.
func transcodePath(taskID, format string) string {
	return filepath.Join(storage.TempDir(), taskID+".to."+format)
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

	// Gỡ hạn ghi cho riêng luồng này.
	//
	// Server đặt WriteTimeout 120 giây để một client ngậm kết nối không giữ được tài nguyên
	// mãi. Nhưng hạn đó tính cho cả phản hồi, mà phản hồi ở đây kéo dài đúng bằng công việc
	// tổng hợp — nên mọi job quá hai phút bị net/http cắt ngang giữa chừng, đúng loại job mà
	// SSE sinh ra để phục vụ. Trình duyệt thấy luồng vỡ rồi tự kết nối lại, tạo thêm một
	// luồng nữa cũng sẽ bị cắt.
	//
	// Bỏ hạn ở đây không mất lớp bảo vệ: vòng lặp dưới thoát ngay khi r.Context() huỷ, tức
	// là khi client ngắt kết nối.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("SSE task %s: không gỡ được hạn ghi (%v); luồng sẽ dừng khi WriteTimeout tới", taskID, err)
	}

	ch := task.Subscribe()
	defer task.Unsubscribe(ch)

	// Gửi sự kiện khởi tạo ban đầu
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
