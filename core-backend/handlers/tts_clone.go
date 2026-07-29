package handlers

import (
	"context"
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

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TTSCloneHandler xử lý các API liên quan đến Voice Cloning (Tải mẫu giọng mẫu, quản lý giọng và tổng hợp tiếng nói theo mẫu giọng).
type TTSCloneHandler struct {
	TTSClient *client.CoreTTSClient
}

// NewTTSCloneHandler khởi tạo TTSCloneHandler với CoreTTSClient.
func NewTTSCloneHandler(ttsClient *client.CoreTTSClient) *TTSCloneHandler {
	return &TTSCloneHandler{TTSClient: ttsClient}
}

// UploadVoice tải file âm thanh mẫu (.wav, .mp3) để nhân bản (clone) giọng nói lâu dài cho tài khoản người dùng.
func (h *TTSCloneHandler) UploadVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	err := r.ParseMultipartForm(32 << 20) // Đọc Form 32MB
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi đọc form upload"})
		return
	}

	name := r.FormValue("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Thiếu tên giọng"})
		return
	}

	modelID := r.FormValue("model_id")
	if modelID == "" {
		modelID = "clone"
	}

	gender := r.FormValue("gender")
	region := r.FormValue("region")
	style := r.FormValue("style")

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Thiếu file âm thanh"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi đọc file"})
		return
	}

	// 1. Lưu file mẫu vào thư mục lưu trữ người dùng phân tầng theo model_id
	userDir := filepath.Join("storage", modelID, user.ID, "voice")
	_ = os.MkdirAll(userDir, 0755)

	cloneID := uuid.NewString()
	ext := "wav"
	if idx := strings.LastIndex(header.Filename, "."); idx != -1 {
		ext = header.Filename[idx+1:]
	}
	filePath := filepath.Join(userDir, fmt.Sprintf("%s.%s", cloneID, ext))
	_ = os.WriteFile(filePath, fileBytes, 0644)

	// 2. Gửi file sang Core TTS Service (Python AI engine) để trích xuất Feature Embeddings
	res, err := h.TTSClient.CloneVoice(fileBytes, header.Filename, name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	coreCloneID := cloneID
	if vID, ok := res["voice_id"].(string); ok && vID != "" {
		coreCloneID = vID
	}

	// 3. Lưu thông tin giọng nhân bản vào PostgreSQL qua sqlc
	params := sqlc.CreateUserVoiceParams{
		ID:       coreCloneID,
		UserID:   user.ID,
		ModelID:  modelID,
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
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi lưu DB voice"})
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"clone_id": coreCloneID,
		"message":  "Clone giọng thành công!",
	})
}

// UploadTempVoice tải giọng mẫu tạm thời (không lưu vào lịch sử DB).
func (h *TTSCloneHandler) UploadTempVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi đọc form upload"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Thiếu file âm thanh"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi đọc file"})
		return
	}

	res, err := h.TTSClient.CloneVoice(fileBytes, header.Filename, "temp_voice")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	cloneID := fmt.Sprintf("temp_%s", uuid.NewString())
	if vID, ok := res["voice_id"].(string); ok && vID != "" {
		cloneID = vID
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"clone_id": cloneID,
		"message":  "Nạp giọng tạm thành công!",
	})
}

// UserVoiceResponse cấu trúc phản hồi danh sách giọng nhân bản của người dùng.
type UserVoiceResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Gender    *string `json:"gender"`
	Region    *string `json:"region"`
	Style     *string `json:"style"`
	CreatedAt string  `json:"created_at"`
}

// GetUserVoices lấy danh sách tất cả các giọng nhân bản của người dùng hiện tại (lọc theo model_id nếu có).
func (h *TTSCloneHandler) GetUserVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	modelID := r.URL.Query().Get("model_id")
	var voices []sqlc.UserVoice
	var err error

	if modelID != "" {
		voices, err = db.Queries.ListUserVoicesByModel(r.Context(), sqlc.ListUserVoicesByModelParams{
			UserID:  user.ID,
			ModelID: modelID,
		})
	} else {
		voices, err = db.Queries.ListUserVoices(r.Context(), user.ID)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi CSDL"})
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
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(res)
}

// DeleteUserVoice xóa một giọng nhân bản khỏi CSDL, đĩa cứng và AI Engine.
func (h *TTSCloneHandler) DeleteUserVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	cloneID := chi.URLParam(r, "clone_id")
	voice, err := db.Queries.GetUserVoiceByID(r.Context(), sqlc.GetUserVoiceByIDParams{
		ID:     cloneID,
		UserID: user.ID,
	})
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Không tìm thấy giọng"})
		return
	}

	if voice.FilePath != "" && os.Getenv("PRESERVE_FILES") == "" {
		_ = os.Remove(voice.FilePath)
	}

	_ = db.Queries.DeleteUserVoice(r.Context(), sqlc.DeleteUserVoiceParams{
		ID:     cloneID,
		UserID: user.ID,
	})
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Đã xóa giọng"})
}

// CloneSynthesizeRequest cấu trúc yêu cầu tổng hợp tiếng nói từ giọng nhân bản.
type CloneSynthesizeRequest struct {
	Text        string  `json:"text"`
	CloneID     *string `json:"clone_id"`
	Voice       *string `json:"voice"`
	Speed       float64 `json:"speed"`
	JobID       *string `json:"job_id"`
	ChunkIndex  *int    `json:"chunk_index"`
	TotalChunks *int    `json:"total_chunks"`
	TaskID      *string `json:"task_id"`
}

// Synthesize thực hiện tổng hợp tiếng nói bất đồng bộ (Asynchronous Background Task) dựa trên giọng nhân bản.
func (h *TTSCloneHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	var req CloneSynthesizeRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Văn bản trống"})
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
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Cần truyền clone_id hoặc voice"})
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

	// Gọi goroutine xử lý bất đồng bộ kết nối AI Engine
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

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"task_id": taskID,
	})
}
