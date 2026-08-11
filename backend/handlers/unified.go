package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"backend/client"
	"backend/db"
	"backend/db/sqlc"
	"backend/queue"
	"backend/state"
	"backend/synth"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// UnifiedSynthesizeRequest là cấu trúc DTO duy nhất đại diện cho bất kỳ yêu cầu tổng hợp tiếng nói nào.
type UnifiedSynthesizeRequest struct {
	Text        string   `json:"text"`
	Voice       string   `json:"voice"`
	Engine      string   `json:"engine"` // "standard" | "fast" | "clone"
	Speed       float64  `json:"speed"`
	Pitch       *float64 `json:"pitch"`
	Emotion     *string  `json:"emotion"`
	JobID       *string  `json:"job_id"`
	ChunkIndex  *int     `json:"chunk_index"`
	TotalChunks *int     `json:"total_chunks"`
	TaskID      *string  `json:"task_id"`
}

// UnifiedVoiceResponse cấu trúc gọn tối giản cho Frontend: ID, Name, Descriptions.
type UnifiedVoiceResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt string            `json:"created_at,omitempty"`
}

type UnifiedHandler struct {
	TTSClient *client.CoreTTSClient
}

func NewUnifiedHandler(ttsClient *client.CoreTTSClient) *UnifiedHandler {
	return &UnifiedHandler{TTSClient: ttsClient}
}

// GetVoices lấy danh sách giọng đọc đơn giản hóa theo model_id
func (h *UnifiedHandler) GetVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := currentUser(w, r)
	if !ok {
		return
	}

	modelID := chi.URLParam(r, "model_id")
	if modelID == "" {
		modelID = r.URL.Query().Get("model_id")
	}
	if modelID == "" {
		modelID = r.URL.Query().Get("mode")
	}
	modelID = strings.ToLower(strings.TrimSpace(modelID))

	unifiedList := []UnifiedVoiceResponse{}

	// 1. Lấy danh sách giọng Preset từ AI Engine nếu Mode hỗ trợ
	m := state.GlobalManifestState.Get()
	supportsPreset := true
	if m != nil && modelID != "" && modelID != "all" {
		supportsPreset = m.ResolveCapabilities(modelID).SupportsPresetVoices
	}

	if supportsPreset {
		// Giọng preset là thứ thay đổi hiếm, nhưng trước đây mỗi lần mở voice picker là một
		// lượt HTTP tới engine (timeout 60s) — mở kéo 4 mode là 4 giây hệt như treo. Cache theo
		// mode; hai lớp hết hạn (TTL + manifest version) nằm trong voice_cache.go.
		presetVoices, cached := getCachedPresetVoices(modelID, time.Now())
		if !cached {
			var fetchErr error
			presetVoices, fetchErr = h.TTSClient.GetVoices(modelID)
			if fetchErr == nil {
				storeCachedPresetVoices(modelID, presetVoices, time.Now())
			}
		}
		for _, v := range presetVoices {
			unifiedList = append(unifiedList, UnifiedVoiceResponse{
				ID:       v.ID,
				Name:     v.Name,
				Metadata: v.Metadata,
			})
		}
	}

	// 2. Lấy danh sách giọng Clone cá nhân của User từ PostgreSQL
	var userVoices []sqlc.UserVoice
	var err error

	if modelID != "" && modelID != "all" {
		userVoices, err = db.Queries.ListUserVoicesByModel(r.Context(), sqlc.ListUserVoicesByModelParams{
			UserID:  user.ID,
			ModelID: modelID,
		})
	} else {
		userVoices, err = db.Queries.ListUserVoices(r.Context(), user.ID)
	}

	if err == nil {
		for _, v := range userVoices {
			meta := map[string]string{}
			if len(v.Metadata) > 0 && string(v.Metadata) != "{}" {
				_ = sonic.Unmarshal(v.Metadata, &meta)
			}

			createdStr := ""
			if v.CreatedAt.Valid {
				createdStr = v.CreatedAt.Time.Format("2006-01-02T15:04:05Z")
			}

			unifiedList = append(unifiedList, UnifiedVoiceResponse{
				ID:        v.ID,
				Name:      v.Name,
				Metadata:  meta,
				CreatedAt: createdStr,
			})
		}
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(unifiedList)
}

// Synthesize là Universal Gateway Endpoint xử lý mọi yêu cầu sinh âm thanh bất đồng bộ có Validate Manifest tự động.
func (h *UnifiedHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := currentUser(w, r)
	if !ok {
		return
	}

	var req UnifiedSynthesizeRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Invalid text input"})
		return
	}

	if req.Speed <= 0 {
		req.Speed = 1.0
	}

	urlModelID := chi.URLParam(r, "model_id")
	if urlModelID == "" {
		urlModelID = chi.URLParam(r, "mode")
	}
	if urlModelID != "" {
		req.Engine = urlModelID
	} else if req.Engine == "" {
		m := state.GlobalManifestState.Get()
		if m != nil && len(m.SupportedModes) > 0 {
			req.Engine = m.SupportedModes[0].ID
		} else {
			req.Engine = "standard"
		}
	}

	// 1. Universal Validation Gate: Kiểm tra xem Request có tuân thủ Manifest của Engine không
	if err := state.GlobalManifestState.ValidateRequest(req.Text, req.Speed, req.Engine); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	if err := state.GlobalManifestState.ValidatePitch(req.Pitch, req.Engine); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	if err := state.GlobalManifestState.ValidateEmotion(req.Emotion, req.Engine); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	taskID := uuid.NewString()
	if req.TaskID != nil && *req.TaskID != "" {
		if !safeTaskID(*req.TaskID) {
			writeError(w, http.StatusBadRequest, "Invalid task ID")
			return
		}
		taskID = *req.TaskID
	}

	jobID := taskID
	if req.JobID != nil && *req.JobID != "" {
		jobID = *req.JobID
	}

	chunkIndex := 0
	if req.ChunkIndex != nil {
		chunkIndex = *req.ChunkIndex
	}

	totalChunks := 1
	if req.TotalChunks != nil && *req.TotalChunks > 0 {
		totalChunks = *req.TotalChunks
	}

	audioParams := db.JobAudioParams{
		Speed:   req.Speed,
		Pitch:   req.Pitch,
		Emotion: req.Emotion,
	}

	// Bản ghi job/chunk là thứ khiến task này về sau truy vấn lại được: lịch sử đọc từ đây, và
	// ownsTask dựa vào nó để biết chủ sở hữu khi task không còn trong RAM. Bỏ lỗi ở đây nghĩa
	// là job vẫn chạy, âm thanh vẫn sinh, nhưng không có gì ghi lại — người dùng mất nó khỏi
	// lịch sử, và sau một lần khởi động lại thì không ai chứng minh được task đó của mình.
	//
	// Dừng luôn thay vì chạy tiếp: một lượt tổng hợp không ai lấy lại được chỉ tiêu tốn GPU.
	if err := db.RegisterJobAndChunk(context.Background(), user.ID, jobID, req.Engine, req.Voice, audioParams, totalChunks, taskID, chunkIndex, req.Text); err != nil {
		if errors.Is(err, db.ErrJobNotOwned) {
			writeError(w, http.StatusForbidden, "Forbidden")
			return
		}
		log.Printf("Không ghi được job %s / chunk %s: %v", jobID, taskID, err)
		writeError(w, http.StatusInternalServerError, "Không khởi tạo được yêu cầu tổng hợp")
		return
	}

	// Chỉ dựng task trong RAM SAU khi DB đã nhận: task_id do client gửi lên, nên GetOrCreate
	// trước lúc này cho phép một người gắn tên mình lên task_id của người khác. SetOwner không
	// ghi đè, nhưng nó chỉ giữ được điều đó khi task còn trong RAM tiến trình này — task đã bị
	// Cleanup thu hồi, hoặc đang nằm ở replica khác, sẽ dựng lại thành một bản không chủ và
	// người gọi sau chiếm được. Chèn chunk ở trên đã hỏng vì trùng khoá chính, nhưng phần ghi
	// vào RAM thì không có ai hoàn tác.
	//
	// Tới đây thì DB đã xác nhận taskID này là chunk mới của một job thuộc người gọi.
	taskItem := state.GlobalTaskManager.GetOrCreate(taskID)
	taskItem.SetOwner(user.ID)

	job := queue.Job{
		TaskID:  taskID,
		UserID:  user.ID,
		Text:    req.Text,
		Voice:   req.Voice,
		Engine:  req.Engine,
		Speed:   req.Speed,
		Pitch:   req.Pitch,
		Emotion: req.Emotion,
	}

	// Xếp hàng để worker nhặt. Không có Redis thì chạy ngay trong tiến trình này.
	//
	// Nhánh dự phòng giữ cho một triển khai chỉ có web vẫn tổng hợp được — cùng lý do
	// TaskManager và rate limiter đều có bản chạy bằng RAM. Nó dùng chung đúng hàm synth.Run
	// mà worker gọi, nên hai đường không thể trôi ra khỏi nhau.
	//
	// Qua GoLocal chứ không `go` trần: lượt này phải được ghi nhận để lúc tắt máy còn chờ nó
	// chạy nốt, giống wg.Wait() bên worker.
	if err := queue.Enqueue(r.Context(), job); err != nil {
		if !errors.Is(err, queue.ErrNoRedis) {
			log.Printf("Không xếp được job %s vào hàng đợi: %v — chạy tại chỗ", taskID, err)
		}
		synth.GoLocal(h.TTSClient, job)
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"task_id": taskID,
	})
}
