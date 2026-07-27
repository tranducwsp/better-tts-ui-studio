package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"core-backend/client"
	"core-backend/db"
	"core-backend/middleware"
	"core-backend/state"

	"github.com/google/uuid"
)

type TTSStandardHandler struct {
	TTSClient *client.CoreTTSClient
}

func NewTTSStandardHandler(ttsClient *client.CoreTTSClient) *TTSStandardHandler {
	return &TTSStandardHandler{TTSClient: ttsClient}
}

type StandardSynthesizeRequest struct {
	Text        string   `json:"text"`
	Voice       string   `json:"voice"`
	Speed       float64  `json:"speed"`
	JobID       *string  `json:"job_id"`
	ChunkIndex  *int     `json:"chunk_index"`
	TotalChunks *int     `json:"total_chunks"`
	TaskID      *string  `json:"task_id"`
}

func (h *TTSStandardHandler) GetVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	voices, err := h.TTSClient.GetVoices()
	if err == nil && len(voices) > 0 {
		names := make([]string, len(voices))
		for i, v := range voices {
			names[i] = v.Name
		}
		_ = json.NewEncoder(w).Encode(names)
		return
	}

	_ = json.NewEncoder(w).Encode([]string{"Minh Đức", "Hoài Mỹ"})
}

func (h *TTSStandardHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	var req StandardSynthesizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Văn bản trống"})
		return
	}

	if req.Voice == "" {
		req.Voice = "Phạm Tuyên"
	}
	if req.Speed <= 0 {
		req.Speed = 1.0
	}

	taskID := uuid.NewString()
	if req.TaskID != nil && *req.TaskID != "" {
		taskID = *req.TaskID
	}

	taskItem := state.GlobalTaskManager.GetOrCreate(taskID)

	if req.JobID != nil && *req.JobID != "" && req.ChunkIndex != nil && req.TotalChunks != nil {
		_ = db.RegisterJobAndChunk(context.Background(), user.ID, *req.JobID, "standard", req.Voice, req.Speed, *req.TotalChunks, taskID, *req.ChunkIndex, req.Text)
	}

	go func() {
		bgCtx := context.Background()
		audioBytes, err := h.TTSClient.Synthesize(req.Text, req.Voice, req.Speed, "standard")
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

	_ = json.NewEncoder(w).Encode(map[string]string{
		"task_id": taskID,
	})
}
