package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"core-backend/client"
	"core-backend/db"
	"core-backend/middleware"
	"core-backend/state"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
)

// UnifiedSynthesizeRequest là cấu trúc DTO duy nhất đại diện cho bất kỳ yêu cầu tổng hợp tiếng nói nào.
type UnifiedSynthesizeRequest struct {
	Text        string  `json:"text"`
	Voice       string  `json:"voice"`
	Engine      string  `json:"engine"` // "standard" | "fast" | "clone"
	Speed       float64 `json:"speed"`
	JobID       *string `json:"job_id"`
	ChunkIndex  *int    `json:"chunk_index"`
	TotalChunks *int    `json:"total_chunks"`
	TaskID      *string `json:"task_id"`
}

// UnifiedVoiceResponse cấu trúc chung phản hồi danh sách giọng đọc cho Frontend.
type UnifiedVoiceResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"` // "standard" | "fast" | "clone"
	Gender    *string `json:"gender,omitempty"`
	Region    *string `json:"region,omitempty"`
	Style     *string `json:"style,omitempty"`
	CreatedAt string  `json:"created_at,omitempty"`
}

type UnifiedHandler struct {
	TTSClient *client.CoreTTSClient
}

func NewUnifiedHandler(ttsClient *client.CoreTTSClient) *UnifiedHandler {
	return &UnifiedHandler{TTSClient: ttsClient}
}

// GetVoices hợp nhất toàn bộ Giọng Preset của AI Engine và Giọng Clone của User vào 1 API duy nhất.
func (h *UnifiedHandler) GetVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	unifiedList := []UnifiedVoiceResponse{}

	// 1. Lấy danh sách giọng Preset từ AI Engine
	presetVoices, err := h.TTSClient.GetVoices()
	if err == nil {
		for _, v := range presetVoices {
			unifiedList = append(unifiedList, UnifiedVoiceResponse{
				ID:   v.ID,
				Name: v.Name,
				Type: "standard",
			})
		}
	}

	// 2. Lấy danh sách giọng Clone cá nhân của User từ PostgreSQL
	userVoices, err := db.Queries.ListUserVoices(r.Context(), user.ID)
	if err == nil {
		for _, v := range userVoices {
			var genderPtr, regionPtr, stylePtr *string
			if v.Gender.Valid {
				genderPtr = &v.Gender.String
			}
			if v.Region.Valid {
				regionPtr = &v.Region.String
			}
			if v.Style.Valid {
				stylePtr = &v.Style.String
			}

			createdStr := ""
			if v.CreatedAt.Valid {
				createdStr = v.CreatedAt.Time.Format("2006-01-02T15:04:05Z")
			}

			unifiedList = append(unifiedList, UnifiedVoiceResponse{
				ID:        v.ID,
				Name:      v.Name,
				Type:      "clone",
				Gender:    genderPtr,
				Region:    regionPtr,
				Style:     stylePtr,
				CreatedAt: createdStr,
			})
		}
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(unifiedList)
}

// Synthesize là Universal Gateway Endpoint xử lý mọi yêu cầu sinh âm thanh bất đồng bộ có Validate Manifest tự động.
func (h *UnifiedHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	var req UnifiedSynthesizeRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Văn bản không hợp lệ"})
		return
	}

	if req.Speed <= 0 {
		req.Speed = 1.0
	}

	if req.Engine == "" {
		req.Engine = "standard"
	}

	// 1. Universal Validation Gate: Kiểm tra xem Request có tuân thủ Manifest của Engine không
	if err := state.GlobalManifestState.ValidateRequest(req.Text, req.Speed); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	taskID := uuid.NewString()
	if req.TaskID != nil && *req.TaskID != "" {
		taskID = *req.TaskID
	}

	taskItem := state.GlobalTaskManager.GetOrCreate(taskID)

	if req.JobID != nil && *req.JobID != "" && req.ChunkIndex != nil && req.TotalChunks != nil {
		_ = db.RegisterJobAndChunk(context.Background(), user.ID, *req.JobID, req.Engine, req.Voice, req.Speed, *req.TotalChunks, taskID, *req.ChunkIndex, req.Text)
	}

	// Async Task Worker Goroutine
	go func() {
		bgCtx := context.Background()
		audioBytes, err := h.TTSClient.Synthesize(req.Text, req.Voice, req.Speed, req.Engine)
		if err != nil {
			taskItem.Notify(state.TaskUpdate{
				Status:   "error",
				Progress: taskItem.Progress,
				Error:    err.Error(),
			})
			if req.JobID != nil && *req.JobID != "" {
				errMsg := err.Error()
				_ = db.UpdateChunkStatus(bgCtx, taskID, "error", nil, &errMsg)
			}
			return
		}

		taskItem.AudioWAV = audioBytes
		taskItem.AudioMP3 = audioBytes

		_ = os.MkdirAll("storage/temp", 0755)
		filePath := filepath.Join("storage/temp", fmt.Sprintf("%s.wav", taskID))
		_ = os.WriteFile(filePath, audioBytes, 0644)

		if req.JobID != nil && *req.JobID != "" {
			_ = db.UpdateChunkStatus(bgCtx, taskID, "done", &filePath, nil)
		}

		taskItem.Notify(state.TaskUpdate{
			Status:   "done",
			Progress: 100,
		})
	}()

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"task_id": taskID,
	})
}
