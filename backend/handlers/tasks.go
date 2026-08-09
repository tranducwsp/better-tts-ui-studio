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

// TasksHandler xử lý việc kiểm tra tiến độ, stream Server-Sent Events (SSE), hủy task và tải file audio.
type TasksHandler struct {
	presignTTL time.Duration
}

// NewTasksHandler khởi tạo TasksHandler.
func NewTasksHandler(ttl ...time.Duration) *TasksHandler {
	presignTTL := 60 * time.Second
	if len(ttl) > 0 && ttl[0] > 0 {
		presignTTL = ttl[0]
	}
	return &TasksHandler{presignTTL: presignTTL}
}

// ownsTask kiểm tra người gọi có quyền trên task này không, và đã ghi phản hồi lỗi nếu không.
//
// Trước đây bốn handler dưới đây chỉ tra task_id rồi trả kết quả. task_id là UUID nên khó
// đoán, nhưng nó lộ ra trong lịch sử (ChunkItemResponse.TaskID) và do client tự gửi khi tổng
// hợp, nên "khó đoán" không phải là kiểm soát truy cập.
//
// Chủ sở hữu lấy từ TaskItem trước, chỉ hỏi DB khi task trong RAM không mang theo trường đó
// — tức task được dựng lại từ Redis sau một lần khởi động lại. Polling trạng thái và SSE là
// hai đường đi nóng nhất của một job đang chạy; một truy vấn join cho mỗi lần hỏi tiến độ là
// cái giá không cần trả cho thông tin mà tiến trình này đã biết.
//
// Task không tìm được chủ ở cả hai nơi bị từ chối: thà chặn một task hợp lệ còn hơn mở mọi
// task cho mọi người vì một bản ghi thiếu.
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

// GetTaskStatus lấy trạng thái (status, progress) của một Task bất đồng bộ qua task_id.
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

	// Ghi trạng thái xuống DB ngay, không chờ worker. Trước đây chỉ có Cancel ở tầng state: job
	// còn đang xếp hàng thì không ai đặt chunk về 'cancelled', bảng tts_chunks cứ đứng ở
	// 'processing' mãi dù người dùng đã bấm dừng và giao diện đã hiện "cancelled".
	//
	// Dùng context.Background() thay vì r.Context(): người dùng có thể đã ngắt kết nối ngay sau
	// khi gửi lệnh dừng, và bản ghi này vẫn phải có hiệu lực dù client không nghe phản hồi.
	// Update là dạng có điều kiện (chỉ chuyển từ pending/processing) nên không đè 'done'/'error'
	// nếu worker thắng cuộc đua.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.CancelChunk(ctx, taskID); err != nil {
		log.Printf("Task %s: huỷ ở tầng state được nhưng không ghi được trạng thái vào DB: %v", taskID, err)
	}

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
	if !ownsTask(w, r, taskID) {
		return
	}
	if format != "" && !audio.CanTranscode(format) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Audio format '%s' is not supported", format))
		return
	}

	// Hỏi kho trước khi chạm tới RAM.
	//
	// Với kho phát được URL, đường nhanh nhất là chuyển hướng client sang thẳng đó — và lúc
	// ấy tiến trình này không cần bytes chút nào. Đọc TaskManager trước sẽ phá đúng điều đó:
	// Get() nạp bù âm thanh từ Redis vào RAM khi thấy RAM rỗng, nên tới đây source luôn khác
	// nil và nhánh chuyển hướng không bao giờ chạy. Đó là lý do bản đầu của thay đổi này vẫn
	// trả 200 kèm toàn bộ tệp thay vì 302.
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

	// Bản đã chuyển mã của lần tải trước nằm cạnh bản gốc trong kho, nên lần này khỏi gọi
	// ffmpeg. Bộ quét dọn thu hồi cả hai theo cùng một chính sách.
	if format != "" {
		if b, err := storage.Global.Get(r.Context(), storage.TranscodeKey(taskID, format)); err == nil && len(b) > 0 {
			writeAudio(w, b, format)
			return
		}
	}

	// Task có thể đã bị dọn khỏi RAM; đối tượng trong kho mang phần mở rộng là định dạng gốc.
	//
	// Dò bằng Exists trước khi tải: khi client hỏi đúng định dạng gốc — trường hợp thường gặp
	// nhất — không cần nạp bytes vào tiến trình này chút nào.
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
	// Bản trong kho chỉ là bộ nhớ đệm cho lần tải sau; ghi thất bại chỉ có nghĩa là lần sau
	// chạy lại ffmpeg, nên không cần làm hỏng phản hồi đang thành công. Vẫn log để đĩa đầy
	// không biểu hiện thành "sao dạo này tải chậm".
	transKey := storage.TranscodeKey(taskID, format)
	if err := storage.Global.Put(r.Context(), transKey, bytes.NewReader(converted)); err != nil {
		log.Printf("Không cất được bản chuyển mã %s.%s: %v", taskID, format, err)
		// Không cất được thì không ký được: URL sẽ trỏ vào đối tượng không tồn tại.
		writeAudio(w, converted, format)
		return
	}
	if h.serveFromStore(w, r, transKey, format) {
		return
	}
	writeAudio(w, converted, format)
}

// taskSourceFormat đọc định dạng Engine đã sinh, không kéo theo bytes.
//
// Tách khỏi TaskManager.Get vì hàm đó nạp bù âm thanh từ Redis vào RAM như một tác dụng phụ —
// hữu ích cho đường phục vụ bytes, nhưng ở đây thì đúng là thứ cần tránh.
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

// presignTTL là thời hạn mặc định của URL tải trực tiếp khi handler được dựng ngoài bootstrap.
// Production truyền giá trị từ Config; default chỉ giữ các test/consumer cũ chạy an toàn.
const defaultPresignTTL = 60 * time.Second

// serveFromStore trả âm thanh cho client, và cho biết đã trả được chưa.
//
// Với kho phát được URL (S3), gửi 302 tới URL đã ký: client tải thẳng từ kho, nên backend
// không còn là ống dẫn. Đo được trước khi đổi: một tệp 563 KB đi qua backend 1180 KB — vào
// một lần rồi ra một lần — và nằm trọn trong RAM suốt lượt tải.
//
// Với kho không phát được URL (đĩa cục bộ), trả về false để người gọi đi đường cũ: đọc bytes
// rồi ghi vào response.
func (h *TasksHandler) serveFromStore(w http.ResponseWriter, r *http.Request, key, format string) bool {
	ps, ok := storage.Global.(storage.Presigner)
	if !ok {
		return false
	}

	url, err := ps.PresignGet(r.Context(), key, h.presignTTL)
	if err != nil {
		// Ký hỏng không phải lý do để từ chối người dùng: đường đọc bytes vẫn còn đó.
		log.Printf("Không ký được URL cho %s: %v — trả về qua backend", key, err)
		return false
	}

	// 302 chứ không phải 301: URL này hết hạn sau presignTTL, nên không được cache lại như
	// một chỗ ở lâu dài.
	w.Header().Set("Content-Disposition", `attachment; filename="tts_studio_audio.`+format+`"`)
	http.Redirect(w, r, url, http.StatusFound)
	return true
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

// writeAudio ghi dữ liệu âm thanh kèm Content-Type và tên tập tin đúng định dạng.
func writeAudio(w http.ResponseWriter, data []byte, format string) {
	w.Header().Set("Content-Type", audio.MimeType(format))
	w.Header().Set("Content-Disposition", `attachment; filename="tts_studio_audio.`+format+`"`)
	_, _ = w.Write(data)
}

// StreamTaskProgress truyền dữ liệu tiến độ thời gian thực (Real-time SSE Stream) qua kết nối HTTP Persistent/Event-Stream.
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
