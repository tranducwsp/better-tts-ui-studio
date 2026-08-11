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

// firstCloningMode returns the first Mode declared by the Manifest as supporting cloning,
// used when the client does not send model_id. Returns an empty string if the Manifest is
// not loaded or no Mode supports cloning — better than assigning a made-up name that
// cannot be queried later.
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

// reservedVoiceFields are fields reserved by the system, not stored in the JSONB metadata.
var reservedVoiceFields = map[string]bool{
	"name": true, "model_id": true, "file": true,
}

// extraMetadata collects every form field outside the dedicated columns into JSON.
//
// voice_metadata_schema allows the Engine to declare arbitrary fields; the platform cannot
// know their names in advance, so the storage must be schema-less. Returns "{}" when there
// is nothing extra, because the column is declared NOT NULL DEFAULT '{}'.
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

// TTSCloneHandler handles Voice Cloning related APIs (uploading voice samples, managing
// voices, and synthesizing speech with cloned voices).
type TTSCloneHandler struct {
	TTSClient *client.CoreTTSClient
	Config    *config.Config
}

// NewTTSCloneHandler initializes a TTSCloneHandler with a CoreTTSClient and configuration.
func NewTTSCloneHandler(ttsClient *client.CoreTTSClient, cfg *config.Config) *TTSCloneHandler {
	return &TTSCloneHandler{TTSClient: ttsClient, Config: cfg}
}

// UploadVoice uploads a reference audio file to permanently clone a voice for the user's account.
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
	// Parts spilled to disk (files > multipartMemoryLimit) must be cleaned up after streaming
	// finishes. Called at the end of the handler, after every stream has been read; parts in
	// RAM only have their memory reclaimed by RemoveAll, nothing leaked.
	defer cleanupMultipartForm(r)

	// Check before reading the file: this is the cheapest check, and swallowing a large file
	// into RAM only to reject it for missing a voice name wastes the user's bandwidth for a
	// response already known in advance.
	name := r.FormValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "Voice name is required")
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

	// A full storage or exhausted quota would previously continue and report "Voice cloned
	// successfully!", while writing a record pointing to a non-existent file — the user only
	// discovers it when selecting that voice, on a different occasion and with an unrelated
	// error message.
	if err := storage.Global.Put(r.Context(), voiceKey, upload.File); err != nil {
		log.Printf("Failed to write voice file %s: %v", voiceKey, err)
		writeError(w, http.StatusInternalServerError, "Failed to save voice to storage")
		return
	}

	// storage.Put has fully consumed File; rewind the pointer to send it onward to the engine.
	// multipart.File is also an io.Seeker, no need to reload from scratch.
	if _, err := upload.File.Seek(0, io.SeekStart); err != nil {
		log.Printf("Failed to re-read voice file %s for sending to engine: %v", voiceKey, err)
		_ = storage.Global.Delete(r.Context(), voiceKey)
		writeError(w, http.StatusInternalServerError, "Failed to read voice file")
		return
	}

	// 2. Send file to Core TTS Service to extract feature embeddings
	//
	// The file just written is cleaned up if subsequent steps fail: no record points to it,
	// so leaving it behind is invisible garbage that the scanner (only looking at storage/temp)
	// will never reclaim.
	res, err := h.TTSClient.CloneVoice(upload.File, upload.Filename, name)
	if err != nil {
		_ = storage.Global.Delete(r.Context(), voiceKey)
		// Engine errors go to the log only. Returning err.Error() directly would expose the
		// hostname, port, and internal details of a service the user does not call directly —
		// the rest of the codebase consistently returns generic messages; this was a gap.
		log.Printf("Engine failed to clone voice %q: %v", name, err)
		writeError(w, http.StatusBadGateway, "Failed to clone voice, please try again")
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
		log.Printf("Failed to save voice record %s: %v", coreCloneID, err)
		writeError(w, http.StatusInternalServerError, "Failed to save voice to database")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"clone_id": coreCloneID,
		"message":  "Voice cloned successfully!",
	})
}

// UploadTempVoice uploads a temporary voice sample (not saved to DB history).
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
		log.Printf("Engine failed to load temporary voice: %v", err)
		writeError(w, http.StatusBadGateway, "Failed to load voice, please try again")
		return
	}

	cloneID := fmt.Sprintf("temp_%s", uuid.NewString())
	if vID, ok := res["voice_id"].(string); ok && vID != "" {
		cloneID = vID
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"clone_id": cloneID,
		"message":  "Temporary voice loaded successfully!",
	})
}

// storageKey converts the FilePath value in the DB into a storage key.
//
// Records created before the storage tier was abstracted into an interface carry full system
// paths ("storage/clone/<user>/voice/x.wav"), while newer records carry keys
// ("clone/<user>/voice/x.wav"). Strips exactly one root prefix if present, so both forms
// are deletable — without this step, every voice stored before the change would remain in
// storage forever.
//
// Also converts Windows path separators to "/" because keys always use "/".
func storageKey(filePath string) string {
	key := filepath.ToSlash(filePath)
	if root := filepath.ToSlash(storage.Root()); root != "" {
		key = strings.TrimPrefix(key, strings.TrimSuffix(root, "/")+"/")
	}
	return strings.TrimPrefix(key, "/")
}

// UserVoiceResponse is the response structure for the user's cloned voice list.
type UserVoiceResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata"`
	CreatedAt string            `json:"created_at"`
}

// GetUserVoices retrieves all cloned voices for the current user (filtered by model_id if provided).
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
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Database error"})
		return
	}

	res := make([]UserVoiceResponse, len(voices))
	for i, v := range voices {
		// Decode JSONB metadata into a map
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

// DeleteUserVoice deletes a cloned voice from the database, disk, and AI Engine.
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
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Voice not found"})
		return
	}

	// Delete the record BEFORE the file: if this statement errors, the file is still intact
	// and the user can retry. The reverse order — deleting the file first then ignoring a DB
	// error — leaves a record pointing to a non-existent file, which the UI still lists as a
	// usable voice.
	if err := db.Queries.DeleteUserVoice(r.Context(), sqlc.DeleteUserVoiceParams{
		ID:     cloneID,
		UserID: user.ID,
	}); err != nil {
		log.Printf("Failed to delete voice record %s: %v", cloneID, err)
		writeError(w, http.StatusInternalServerError, "Failed to delete voice")
		return
	}

	// A file that cannot be deleted is just leftover garbage in storage, no longer affecting
	// the user — so log it and report success, because to them the voice has truly disappeared.
	//
	// The FilePath of old records is a system path ("storage/clone/<user>/voice/x.wav") rather
	// than a key; storageKey strips the root portion so that voices stored before the change
	// are still deletable instead of persisting forever.
	if voice.FilePath != "" && !h.Config.PreserveFiles {
		if err := storage.Global.Delete(r.Context(), storageKey(voice.FilePath)); err != nil {
			log.Printf("Voice record %s deleted but file %s remains: %v", cloneID, voice.FilePath, err)
		}
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Voice deleted"})
}
