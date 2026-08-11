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

// UnifiedSynthesizeRequest is the single DTO representing any speech synthesis request.
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

// UnifiedVoiceResponse is a minimal structure for the Frontend: ID, Name, Descriptions.
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

// GetVoices retrieves a simplified list of voices by model_id.
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

	// 1. Fetch preset voices from the AI Engine if the Mode supports them
	m := state.GlobalManifestState.Get()
	supportsPreset := true
	if m != nil && modelID != "" && modelID != "all" {
		supportsPreset = m.ResolveCapabilities(modelID).SupportsPresetVoices
	}

	if supportsPreset {
		// Preset voices change rarely, but previously every voice picker open triggered an HTTP
		// call to the engine (timeout 60s) — opening 4 modes meant 4 seconds of apparent hang.
		// Cached per mode; two expiration layers (TTL + manifest version) live in presetvoicecache.
		presetVoices, cached := presetVoiceCache.Get(modelID, time.Now())
		if !cached {
			var fetchErr error
			presetVoices, fetchErr = h.TTSClient.GetVoices(modelID)
			if fetchErr == nil {
				presetVoiceCache.Store(modelID, presetVoices, time.Now())
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

	// 2. Fetch the user's personal cloned voices from PostgreSQL
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

// Synthesize is the Universal Gateway Endpoint that handles every asynchronous audio
// generation request with automatic Manifest validation.
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

	// 1. Universal Validation Gate: Check whether the request complies with the Engine's Manifest
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

	// The job/chunk record is what makes this task queryable later: history is read from it,
	// and ownsTask relies on it to determine ownership when the task is no longer in RAM.
	// Dropping the error here means the job still runs, audio is still generated, but nothing
	// is recorded — the user loses it from history, and after a restart no one can prove the
	// task belongs to them.
	//
	// Stop immediately rather than continuing: a synthesis run no one can retrieve only wastes GPU.
	if err := db.RegisterJobAndChunk(context.Background(), user.ID, jobID, req.Engine, req.Voice, audioParams, totalChunks, taskID, chunkIndex, req.Text); err != nil {
		if errors.Is(err, db.ErrJobNotOwned) {
			writeError(w, http.StatusForbidden, "Forbidden")
			return
		}
		log.Printf("Failed to write job %s / chunk %s: %v", jobID, taskID, err)
		writeError(w, http.StatusInternalServerError, "Failed to initialize synthesis request")
		return
	}

	// Only create the task in RAM AFTER the DB has accepted it: task_id is supplied by the
	// client, so calling GetOrCreate before this point allows someone to attach their name to
	// another user's task_id. SetOwner does not overwrite, but that only holds while the task
	// is in this process's RAM — a task already reclaimed by Cleanup, or sitting on another
	// replica, will be rebuilt as an ownerless entry and the next caller takes it. The chunk
	// insert above would have failed on a primary key conflict, but nothing rolls back the RAM
	// entry.
	//
	// By this point the DB has confirmed this taskID is a new chunk of a job owned by the caller.
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

		// Enqueue for the worker to pick up. Without Redis, run inline in this process.
		//
		// The fallback branch keeps a web-only deployment able to synthesize — the same reason
		// TaskManager and rate limiter both have in-memory implementations. It uses the exact same
		// synth.Run function that the worker calls, so the two paths cannot drift apart.
		//
		// Uses GoLocal, not a raw `go`: this run must be tracked so the process waits for it to
		// finish during shutdown, analogous to wg.Wait() in the worker.
		if err := queue.Enqueue(r.Context(), job); err != nil {
		if !errors.Is(err, queue.ErrNoRedis) {
			log.Printf("Failed to enqueue job %s: %v — running inline", taskID, err)
		}
		synth.GoLocal(h.TTSClient, job)
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{
		"task_id": taskID,
	})
}
