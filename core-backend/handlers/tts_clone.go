package handlers

import (
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
	"core-backend/storage"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// firstCloningMode trả về Mode đầu tiên mà Manifest khai là hỗ trợ cloning, dùng khi client
// không gửi model_id. Trả về chuỗi rỗng nếu Manifest chưa nạp hoặc không Mode nào hỗ trợ —
// tốt hơn là gán một tên bịa mà về sau không truy vấn lại được.
func firstCloningMode() string {
	m := state.GlobalManifestState.Get()
	if m == nil {
		return ""
	}
	for _, mode := range m.SupportedModes {
		if m.ResolveCapabilities(mode.ID).SupportsCloning {
			return mode.ID
		}
	}
	return ""
}

// reservedVoiceFields là các trường đã có cột riêng trong bảng user_voices, nên không lặp
// lại chúng trong metadata JSONB.
var reservedVoiceFields = map[string]bool{
	"name": true, "gender": true, "region": true, "style": true,
	"model_id": true, "file": true,
}

// extraMetadata gom mọi trường form ngoài các trường đã có cột riêng thành JSON.
//
// voice_metadata_schema cho phép Engine khai bất kỳ trường nào; nền tảng không thể biết
// trước tên chúng, nên chỗ lưu phải là schema-less. Trả về "{}" khi không có gì thêm, vì
// cột được khai NOT NULL DEFAULT '{}'.
func extraMetadata(r *http.Request) []byte {
	if r.MultipartForm == nil {
		return []byte("{}")
	}

	extra := map[string]string{}
	for key, vals := range r.MultipartForm.Value {
		if reservedVoiceFields[key] || len(vals) == 0 || vals[0] == "" {
			continue
		}
		extra[key] = vals[0]
	}
	if len(extra) == 0 {
		return []byte("{}")
	}

	raw, err := sonic.Marshal(extra)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

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

	if err := parseUpload(w, r); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
		return
	}

	name := r.FormValue("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Voice name is required"})
		return
	}

	// Không đoán tên mode: hỏi Manifest xem Mode nào thực sự hỗ trợ cloning. Chuỗi "clone"
	// cứng trước đây khiến giọng của một Engine đặt tên mode là zero_shot_clone bị lưu dưới
	// một model_id không tồn tại, nên sau đó không mode nào liệt kê được nó.
	modelID := r.FormValue("model_id")
	if modelID == "" {
		modelID = firstCloningMode()
	}

	// gender/region/style có cột riêng vì chúng được lọc và hiển thị. Mọi trường khác mà
	// Engine khai trong voice_metadata_schema đi vào cột metadata JSONB — nếu không, một
	// Engine khai năm trường sẽ thấy hai trường biến mất mà người dùng không hay biết.
	gender := r.FormValue("gender")
	region := r.FormValue("region")
	style := r.FormValue("style")
	extra := extraMetadata(r)

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Audio file is required"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to read audio file"})
		return
	}

	// 1. Save reference audio file under storage tier
	userDir := storage.ModeDir(modelID, user.ID)
	_ = os.MkdirAll(userDir, 0755)

	cloneID := uuid.NewString()
	ext := "wav"
	if idx := strings.LastIndex(header.Filename, "."); idx != -1 {
		ext = header.Filename[idx+1:]
	}
	filePath := filepath.Join(userDir, fmt.Sprintf("%s.%s", cloneID, ext))
	_ = os.WriteFile(filePath, fileBytes, 0644)

	// 2. Send file to Core TTS Service to extract feature embeddings
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

	// 3. Save voice record in PostgreSQL via sqlc
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
	params.Metadata = extra

	_, err = db.Queries.CreateUserVoice(r.Context(), params)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Failed to save voice in database"})
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"clone_id": coreCloneID,
		"message":  "Voice cloned successfully!",
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

	if err := parseUpload(w, r); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": err.Error()})
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
