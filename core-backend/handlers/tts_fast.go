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

// TTSFastHandler xử lý các yêu cầu tổng hợp tiếng nói tốc độ cao (Fast TTS Engine).
type TTSFastHandler struct {
	TTSClient *client.CoreTTSClient
	VoiceMap  map[string]string
}

// NewTTSFastHandler khởi tạo TTSFastHandler cùng bảng ánh xạ tên giọng thân thiện sang mã voice code hệ thống.
func NewTTSFastHandler(ttsClient *client.CoreTTSClient) *TTSFastHandler {
	return &TTSFastHandler{
		TTSClient: ttsClient,
		VoiceMap: map[string]string{
			"Hoài Mỹ (Nữ)":       "vi-VN-HoaiMyNeural",
			"Nam Minh (Nam)":     "vi-VN-NamMinhNeural",
			"Hoài Mỹ":            "vi-VN-HoaiMyNeural",
			"Nam Minh":           "vi-VN-NamMinhNeural",
			"vi-VN-HoaiMyNeural": "vi-VN-HoaiMyNeural",
			"vi-VN-NamMinhNeural": "vi-VN-NamMinhNeural",
		},
	}
}

// FastSynthesizeRequest cấu trúc thông số yêu cầu tổng hợp Fast TTS.
type FastSynthesizeRequest struct {
	Text        string   `json:"text"`
	Voice       string   `json:"voice"`
	Speed       float64  `json:"speed"`
	JobID       *string  `json:"job_id"`
	ChunkIndex  *int     `json:"chunk_index"`
	TotalChunks *int     `json:"total_chunks"`
	TaskID      *string  `json:"task_id"`
}

// GetVoices lấy danh sách các giọng Fast TTS hỗ trợ.
func (h *TTSFastHandler) GetVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = sonic.ConfigDefault.NewEncoder(w).Encode([]string{"Hoài Mỹ (Nữ)", "Nam Minh (Nam)"})
}

// Synthesize tổng hợp tiếng nói Fast TTS cực nhanh trong background và trả về audio MP3.
func (h *TTSFastHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	var req FastSynthesizeRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Văn bản trống"})
		return
	}

	if req.Voice == "" {
		req.Voice = "Hoài Mỹ (Nữ)"
	}
	if req.Speed <= 0 {
		req.Speed = 1.0
	}

	voiceCode, exists := h.VoiceMap[req.Voice]
	if !exists {
		voiceCode = req.Voice
	}

	taskID := uuid.NewString()
	if req.TaskID != nil && *req.TaskID != "" {
		taskID = *req.TaskID
	}

	taskItem := state.GlobalTaskManager.GetOrCreate(taskID)

	if req.JobID != nil && *req.JobID != "" && req.ChunkIndex != nil && req.TotalChunks != nil {
		_ = db.RegisterJobAndChunk(context.Background(), user.ID, *req.JobID, "fast", req.Voice, req.Speed, *req.TotalChunks, taskID, *req.ChunkIndex, req.Text)
	}

	go func() {
		bgCtx := context.Background()
		audioBytes, err := h.TTSClient.Synthesize(req.Text, voiceCode, req.Speed, "fast")
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

		taskItem.AudioMP3 = audioBytes
		taskItem.AudioWAV = audioBytes

		_ = os.MkdirAll("storage/temp", 0755)
		filePath := filepath.Join("storage/temp", fmt.Sprintf("%s.mp3", taskID))
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
