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
	"github.com/go-chi/chi/v5"
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

// UnifiedVoiceResponse cấu trúc gọn tối giản cho Frontend: ID, Name, Descriptions.
type UnifiedVoiceResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Descriptions []string `json:"descriptions,omitempty"`
	CreatedAt    string   `json:"created_at,omitempty"`
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
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
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

	// 1. Lấy danh sách giọng Preset từ AI Engine nếu modelID rỗng, "all", "standard" hoặc "fast"
	if modelID == "" || modelID == "all" || modelID == "standard" || modelID == "fast" {
		presetVoices, err := h.TTSClient.GetVoices()
		if err == nil {
			for _, v := range presetVoices {
				vType := strings.ToLower(strings.TrimSpace(v.Type))
				if vType == "" {
					vType = "standard"
				}
				if (modelID == "standard" || modelID == "fast") && vType != modelID {
					continue
				}

				var desc []string
				if v.Gender != "" {
					desc = append(desc, v.Gender)
				}
				if v.Region != "" {
					desc = append(desc, v.Region)
				}
				if v.Style != "" {
					desc = append(desc, v.Style)
				}

				unifiedList = append(unifiedList, UnifiedVoiceResponse{
					ID:           v.ID,
					Name:         v.Name,
					Descriptions: desc,
				})
			}
		}
	}

	// 2. Lấy danh sách giọng Clone cá nhân của User từ PostgreSQL nếu modelID rỗng, "all" hoặc "clone"
	if modelID == "" || modelID == "all" || modelID == "clone" {
		userVoices, err := db.Queries.ListUserVoices(r.Context(), user.ID)
		if err == nil {
			for _, v := range userVoices {
				var desc []string
				if v.Gender.Valid && v.Gender.String != "" {
					desc = append(desc, v.Gender.String)
				}
				if v.Region.Valid && v.Region.String != "" {
					desc = append(desc, v.Region.String)
				}
				if v.Style.Valid && v.Style.String != "" {
					desc = append(desc, v.Style.String)
				}

				createdStr := ""
				if v.CreatedAt.Valid {
					createdStr = v.CreatedAt.Time.Format("2006-01-02T15:04:05Z")
				}

				unifiedList = append(unifiedList, UnifiedVoiceResponse{
					ID:           v.ID,
					Name:         v.Name,
					Descriptions: desc,
					CreatedAt:    createdStr,
				})
			}
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

	urlModelID := chi.URLParam(r, "model_id")
	if urlModelID == "" {
		urlModelID = chi.URLParam(r, "mode")
	}
	if urlModelID != "" {
		req.Engine = urlModelID
	} else if req.Engine == "" {
		req.Engine = "standard"
	}

	// 1. Universal Validation Gate: Kiểm tra xem Request có tuân thủ Manifest của Engine không
	if err := state.GlobalManifestState.ValidateRequest(req.Text, req.Speed, req.Engine); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	taskID := uuid.NewString()
	if req.TaskID != nil && *req.TaskID != "" {
		taskID = *req.TaskID
	}

	taskItem := state.GlobalTaskManager.GetOrCreate(taskID)

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

	_ = db.RegisterJobAndChunk(context.Background(), user.ID, jobID, req.Engine, req.Voice, req.Speed, totalChunks, taskID, chunkIndex, req.Text)

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
			errMsg := err.Error()
			_ = db.UpdateChunkStatus(bgCtx, taskID, "error", nil, &errMsg)
			return
		}

		taskItem.AudioWAV = audioBytes
		taskItem.AudioMP3 = audioBytes

		_ = os.MkdirAll("storage/temp", 0755)
		filePath := filepath.Join("storage/temp", fmt.Sprintf("%s.wav", taskID))
		_ = os.WriteFile(filePath, audioBytes, 0644)

		_ = db.UpdateChunkStatus(bgCtx, taskID, "done", &filePath, nil)

		taskItem.Notify(state.TaskUpdate{
			Status:   "done",
			Progress: 100,
		})
	}()

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"task_id": taskID,
	})
}
