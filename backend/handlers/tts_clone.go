package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"backend/client"
	"backend/config"
	"backend/db"
	"backend/db/sqlc"
	"backend/state"
	"backend/storage"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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

// reservedVoiceFields là các trường do hệ thống dùng riêng, không đi vào metadata JSONB.
var reservedVoiceFields = map[string]bool{
	"name": true, "model_id": true, "file": true,
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
	Config    *config.Config
}

// NewTTSCloneHandler khởi tạo TTSCloneHandler với CoreTTSClient và cấu hình.
func NewTTSCloneHandler(ttsClient *client.CoreTTSClient, cfg *config.Config) *TTSCloneHandler {
	return &TTSCloneHandler{TTSClient: ttsClient, Config: cfg}
}

// UploadVoice tải file âm thanh mẫu để nhân bản (clone) giọng nói lâu dài cho tài khoản người dùng.
func (h *TTSCloneHandler) UploadVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := currentUser(w, r)
	if !ok {
		return
	}

	if err := parseUpload(w, r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Part spill ra đĩa (file > multipartMemoryLimit) cần được xoá sau khi stream xong. Gọi
	// cuối handler, sau khi mọi luồng đã đọc; part nằm trong RAM thì RemoveAll chỉ thu hồi
	// bộ nhớ, không có gì rơi vãi.
	defer cleanupMultipartForm(r)

	// Kiểm trước khi đọc tệp: đây là kiểm rẻ nhất, và nuốt hết một tệp lớn vào RAM để rồi
	// từ chối vì thiếu tên giọng là lãng phí băng thông của người dùng cho một câu trả lời
	// đã biết trước.
	name := r.FormValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "Tên giọng là bắt buộc")
		return
	}

	upload, ok := receiveReferenceAudio(w, r)
	if !ok {
		return
	}
	defer upload.File.Close()

	extra := extraMetadata(r)

	// 1. Save reference audio under the storage tier
	cloneID := uuid.NewString()
	ext := "wav"
	if idx := strings.LastIndex(upload.Filename, "."); idx != -1 {
		ext = upload.Filename[idx+1:]
	}
	voiceKey := storage.VoiceKey(upload.ModelID, user.ID, cloneID+"."+ext)

	// Kho đầy hay hết quota trước đây vẫn đi tiếp và báo "Nhân bản giọng thành công!", đồng
	// thời ghi một bản ghi trỏ tới tệp không tồn tại — người dùng chỉ phát hiện khi chọn dùng
	// giọng đó, ở một lần khác và một thông báo lỗi không liên quan.
	if err := storage.Global.Put(r.Context(), voiceKey, upload.File); err != nil {
		log.Printf("Không ghi được tệp giọng %s: %v", voiceKey, err)
		writeError(w, http.StatusInternalServerError, "Không lưu được giọng vào kho")
		return
	}

	// storage.Put đã đọc hết File; đưa con trỏ về 0 để gửi tiếp sang engine. multipart.File
	// cũng là io.Seeker, không cần nạp lại từ đầu.
	if _, err := upload.File.Seek(0, io.SeekStart); err != nil {
		log.Printf("Không đọc lại được tệp giọng %s để gửi engine: %v", voiceKey, err)
		_ = storage.Global.Delete(r.Context(), voiceKey)
		writeError(w, http.StatusInternalServerError, "Không đọc được tệp giọng")
		return
	}

	// 2. Send file to Core TTS Service to extract feature embeddings
	//
	// Tệp vừa ghi được dọn nếu các bước sau thất bại: không có bản ghi nào trỏ tới nó, nên để
	// lại thì nó là rác vô hình mà bộ quét dọn (chỉ nhìn storage/temp) không bao giờ thu hồi.
	res, err := h.TTSClient.CloneVoice(upload.File, upload.Filename, name)
	if err != nil {
		_ = storage.Global.Delete(r.Context(), voiceKey)
		// Lỗi của Engine chỉ vào log. Trả err.Error() thẳng ra ngoài sẽ lộ tên máy, cổng và
		// chi tiết nội bộ của một dịch vụ mà người dùng không gọi trực tiếp — phần còn lại
		// của mã nguồn đã nhất quán trả thông báo chung, đây là chỗ sót.
		log.Printf("Engine không nhân bản được giọng %q: %v", name, err)
		writeError(w, http.StatusBadGateway, "Không nhân bản được giọng, vui lòng thử lại")
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
		ModelID:  upload.ModelID,
		Name:     name,
		FilePath: voiceKey,
		Metadata: extra,
	}

	_, err = db.Queries.CreateUserVoice(r.Context(), params)
	if err != nil {
		_ = storage.Global.Delete(r.Context(), voiceKey)
		log.Printf("Không lưu được bản ghi giọng %s: %v", coreCloneID, err)
		writeError(w, http.StatusInternalServerError, "Không lưu được giọng vào cơ sở dữ liệu")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"clone_id": coreCloneID,
		"message":  "Nhân bản giọng thành công!",
	})
}

// UploadTempVoice tải giọng mẫu tạm thời (không lưu vào lịch sử DB).
func (h *TTSCloneHandler) UploadTempVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, ok := currentUser(w, r); !ok {
		return
	}

	if err := parseUpload(w, r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanupMultipartForm(r)

	upload, ok := receiveReferenceAudio(w, r)
	if !ok {
		return
	}
	defer upload.File.Close()

	res, err := h.TTSClient.CloneVoice(upload.File, upload.Filename, "temp_voice")
	if err != nil {
		log.Printf("Engine không nạp được giọng tạm: %v", err)
		writeError(w, http.StatusBadGateway, "Không nạp được giọng, vui lòng thử lại")
		return
	}

	cloneID := fmt.Sprintf("temp_%s", uuid.NewString())
	if vID, ok := res["voice_id"].(string); ok && vID != "" {
		cloneID = vID
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"clone_id": cloneID,
		"message":  "Nạp giọng tạm thành công!",
	})
}

// storageKey đổi giá trị FilePath trong DB thành khoá của kho.
//
// Bản ghi tạo trước khi tầng lưu trữ tách thành interface mang đường dẫn hệ thống đầy đủ
// ("storage/clone/<user>/voice/x.wav"), còn bản ghi mới mang khoá ("clone/<user>/voice/x.wav").
// Bỏ đúng một tiền tố gốc nếu có, nên cả hai dạng đều xoá được — không có bước này thì mọi
// giọng lưu trước lần đổi này nằm lại trong kho vĩnh viễn.
//
// Cũng đổi dấu phân cách của Windows sang "/" vì khoá luôn dùng "/".
func storageKey(filePath string) string {
	key := filepath.ToSlash(filePath)
	if root := filepath.ToSlash(storage.Root()); root != "" {
		key = strings.TrimPrefix(key, strings.TrimSuffix(root, "/")+"/")
	}
	return strings.TrimPrefix(key, "/")
}

// UserVoiceResponse cấu trúc phản hồi danh sách giọng nhân bản của người dùng.
type UserVoiceResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata"`
	CreatedAt string            `json:"created_at"`
}

// GetUserVoices lấy danh sách tất cả các giọng nhân bản của người dùng hiện tại (lọc theo model_id nếu có).
func (h *TTSCloneHandler) GetUserVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := currentUser(w, r)
	if !ok {
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
		// Giải mã metadata JSONB thành map
		meta := map[string]string{}
		if len(v.Metadata) > 0 && string(v.Metadata) != "{}" {
			_ = sonic.Unmarshal(v.Metadata, &meta)
		}

		createdStr := ""
		if v.CreatedAt.Valid {
			createdStr = v.CreatedAt.Time.Format("2006-01-02T15:04:05Z")
		}

		res[i] = UserVoiceResponse{
			ID:        v.ID,
			Name:      v.Name,
			Metadata:  meta,
			CreatedAt: createdStr,
		}
	}
	_ = sonic.ConfigDefault.NewEncoder(w).Encode(res)
}

// DeleteUserVoice xóa một giọng nhân bản khỏi CSDL, đĩa cứng và AI Engine.
func (h *TTSCloneHandler) DeleteUserVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := currentUser(w, r)
	if !ok {
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

	// Xoá bản ghi TRƯỚC tệp: nếu câu lệnh này lỗi thì tệp vẫn còn và người dùng thử lại được.
	// Thứ tự ngược lại — xoá tệp trước rồi bỏ qua lỗi DB — để lại một bản ghi trỏ tới tệp
	// không tồn tại, mà giao diện vẫn liệt kê như một giọng dùng được.
	if err := db.Queries.DeleteUserVoice(r.Context(), sqlc.DeleteUserVoiceParams{
		ID:     cloneID,
		UserID: user.ID,
	}); err != nil {
		log.Printf("Không xoá được bản ghi giọng %s: %v", cloneID, err)
		writeError(w, http.StatusInternalServerError, "Không xoá được giọng")
		return
	}

	// Tệp không xoá được chỉ còn là rác trong kho, không còn ảnh hưởng tới người dùng — nên
	// ghi log rồi báo thành công, vì với họ giọng đó đã biến mất thật.
	//
	// FilePath của bản ghi cũ là đường dẫn hệ thống ("storage/clone/<user>/voice/x.wav") chứ
	// không phải khoá; storageKey bóc phần gốc ra để những giọng lưu trước lần đổi này vẫn xoá
	// được thay vì tồn tại mãi.
	if voice.FilePath != "" && !h.Config.PreserveFiles {
		if err := storage.Global.Delete(r.Context(), storageKey(voice.FilePath)); err != nil {
			log.Printf("Bản ghi giọng %s đã xoá nhưng còn tệp %s: %v", cloneID, voice.FilePath, err)
		}
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Đã xóa giọng"})
}
