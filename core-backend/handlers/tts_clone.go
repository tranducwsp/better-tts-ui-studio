package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"core-backend/client"
	"core-backend/db"
	"core-backend/db/sqlc"
	"core-backend/middleware"
	"core-backend/state"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type TTSCloneHandler struct {
	TTSClient *client.CoreTTSClient
}

func NewTTSCloneHandler(ttsClient *client.CoreTTSClient) *TTSCloneHandler {
	return &TTSCloneHandler{TTSClient: ttsClient}
}

func (h *TTSCloneHandler) UploadVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi đọc form upload"})
		return
	}

	name := r.FormValue("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Thiếu tên giọng"})
		return
	}

	gender := r.FormValue("gender")
	region := r.FormValue("region")
	style := r.FormValue("style")

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Thiếu file âm thanh"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi đọc file"})
		return
	}

	userDir := filepath.Join("storage", user.ID, "voice")
	_ = os.MkdirAll(userDir, 0755)

	cloneID := uuid.NewString()
	ext := "wav"
	if idx := strings.LastIndex(header.Filename, "."); idx != -1 {
		ext = header.Filename[idx+1:]
	}
	filePath := filepath.Join(userDir, fmt.Sprintf("%s.%s", cloneID, ext))
	_ = os.WriteFile(filePath, fileBytes, 0644)

	res, err := h.TTSClient.CloneVoice(fileBytes, header.Filename, name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	coreCloneID := cloneID
	if vID, ok := res["voice_id"].(string); ok && vID != "" {
		coreCloneID = vID
	}

	params := sqlc.CreateUserVoiceParams{
		ID:       coreCloneID,
		UserID:   user.ID,
		Name:     name,
		FilePath: filePath,
	}
	if gender != "" {
		params.Gender = pgtype.Text{String: gender, Valid: true}
	}
	if region != "" {
		params.Region = pgtype.Text{String: region, Valid: true}
	}
	if style != "" {
		params.Style = pgtype.Text{String: style, Valid: true}
	}

	_, err = db.Queries.CreateUserVoice(r.Context(), params)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi lưu DB voice"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{
		"clone_id": coreCloneID,
		"message":  "Clone giọng thành công!",
	})
}

func (h *TTSCloneHandler) UploadTempVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi đọc form upload"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Thiếu file âm thanh"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi đọc file"})
		return
	}

	res, err := h.TTSClient.CloneVoice(fileBytes, header.Filename, "temp_voice")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	cloneID := fmt.Sprintf("temp_%s", uuid.NewString())
	if vID, ok := res["voice_id"].(string); ok && vID != "" {
		cloneID = vID
	}

	_ = json.NewEncoder(w).Encode(map[string]string{
		"clone_id": cloneID,
		"message":  "Nạp giọng tạm thành công!",
	})
}

type UserVoiceResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Gender    *string `json:"gender"`
	Region    *string `json:"region"`
	Style     *string `json:"style"`
	CreatedAt string  `json:"created_at"`
}

func (h *TTSCloneHandler) GetUserVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	voices, err := db.Queries.ListUserVoices(r.Context(), user.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi CSDL"})
		return
	}

	res := make([]UserVoiceResponse, len(voices))
	for i, v := range voices {
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

		res[i] = UserVoiceResponse{
			ID:        v.ID,
			Name:      v.Name,
			Gender:    genderPtr,
			Region:    regionPtr,
			Style:     stylePtr,
			CreatedAt: createdStr,
		}
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (h *TTSCloneHandler) DeleteUserVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	cloneID := chi.URLParam(r, "clone_id")
	voice, err := db.Queries.GetUserVoiceByID(r.Context(), sqlc.GetUserVoiceByIDParams{
		ID:     cloneID,
		UserID: user.ID,
	})
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Không tìm thấy giọng"})
		return
	}

	if voice.FilePath != "" && os.Getenv("PRESERVE_FILES") == "" {
		_ = os.Remove(voice.FilePath)
	}

	_, _ = h.TTSClient.DeleteVoice(cloneID)

	_ = db.Queries.DeleteUserVoice(r.Context(), sqlc.DeleteUserVoiceParams{
		ID:     cloneID,
		UserID: user.ID,
	})
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Đã xóa giọng"})
}

type CloneSynthesizeRequest struct {
	Text        string   `json:"text"`
	CloneID     *string  `json:"clone_id"`
	Voice       *string  `json:"voice"`
	Speed       float64  `json:"speed"`
	JobID       *string  `json:"job_id"`
	ChunkIndex  *int     `json:"chunk_index"`
	TotalChunks *int     `json:"total_chunks"`
	TaskID      *string  `json:"task_id"`
}

func (h *TTSCloneHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	var req CloneSynthesizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Văn bản trống"})
		return
	}

	targetCloneID := ""
	if req.CloneID != nil && *req.CloneID != "" {
		targetCloneID = *req.CloneID
	} else if req.Voice != nil && *req.Voice != "" {
		targetCloneID = *req.Voice
	}

	if targetCloneID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Cần truyền clone_id hoặc voice"})
		return
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
		_ = db.RegisterJobAndChunk(context.Background(), user.ID, *req.JobID, "clone", targetCloneID, req.Speed, *req.TotalChunks, taskID, *req.ChunkIndex, req.Text)
	}

	go func() {
		bgCtx := context.Background()
		audioBytes, err := h.TTSClient.Synthesize(req.Text, targetCloneID, req.Speed, "clone")
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
