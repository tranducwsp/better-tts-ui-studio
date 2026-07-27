package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"core-backend/db"
	"core-backend/middleware"
	"core-backend/models"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type HistoryHandler struct{}

func NewHistoryHandler() *HistoryHandler {
	return &HistoryHandler{}
}

type JobSummaryResponse struct {
	JobID      string  `json:"job_id"`
	Engine     string  `json:"engine"`
	Voice      string  `json:"voice"`
	Speed      float64 `json:"speed"`
	Text       string  `json:"text"`
	TimeAgo    string  `json:"time_ago"`
	Progress   string  `json:"progress"`
	IsComplete bool    `json:"is_complete"`
}

func (h *HistoryHandler) GetUserHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	h.getHistoryForUser(w, user.ID)
}

func (h *HistoryHandler) GetUserHistoryAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := chi.URLParam(r, "user_id")
	h.getHistoryForUser(w, userID)
}

func (h *HistoryHandler) getHistoryForUser(w http.ResponseWriter, userID string) {
	var jobs []models.TTSJob
	err := db.DB.Preload("Chunks").Where("user_id = ?", userID).Order("created_at desc").Find(&jobs).Error
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi CSDL"})
		return
	}

	result := make([]JobSummaryResponse, 0, len(jobs))
	now := time.Now()

	for _, job := range jobs {
		shortText := job.Text
		if shortText != "" {
			runes := []rune(shortText)
			if len(runes) > 50 {
				shortText = string(runes[:50]) + "..."
			}
		} else {
			var firstChunk *models.TTSChunk
			for i := range job.Chunks {
				if job.Chunks[i].ChunkIndex == 0 {
					firstChunk = &job.Chunks[i]
					break
				}
			}
			if firstChunk == nil && len(job.Chunks) > 0 {
				firstChunk = &job.Chunks[0]
			}
			if firstChunk != nil {
				shortText = firstChunk.Text
			} else {
				shortText = "Chưa có nội dung"
			}
		}

		uniqueIndexes := make(map[int]bool)
		doneIndexes := make(map[int]bool)
		for _, c := range job.Chunks {
			uniqueIndexes[c.ChunkIndex] = true
			if c.Status == "done" {
				doneIndexes[c.ChunkIndex] = true
			}
		}

		doneChunks := len(doneIndexes)
		actualTotal := job.TotalChunks
		if len(uniqueIndexes) > actualTotal {
			actualTotal = len(uniqueIndexes)
		}

		diff := now.Sub(job.CreatedAt)
		timeAgo := "Vừa xong"
		if diff.Hours() >= 24 {
			days := int(diff.Hours() / 24)
			timeAgo = fmt.Sprintf("%d ngày trước", days)
		} else if diff.Hours() >= 1 {
			hours := int(diff.Hours())
			timeAgo = fmt.Sprintf("%d giờ trước", hours)
		} else if diff.Minutes() >= 1 {
			mins := int(diff.Minutes())
			timeAgo = fmt.Sprintf("%d phút trước", mins)
		}

		result = append(result, JobSummaryResponse{
			JobID:      job.ID,
			Engine:     job.Engine,
			Voice:      job.Voice,
			Speed:      job.Speed,
			Text:       shortText,
			TimeAgo:    timeAgo,
			Progress:   fmt.Sprintf("%d/%d", doneChunks, actualTotal),
			IsComplete: doneChunks == actualTotal,
		})
	}

	_ = json.NewEncoder(w).Encode(result)
}

type ChunkItemResponse struct {
	TaskID     string  `json:"task_id"`
	ChunkIndex int     `json:"chunk_index"`
	AudioPath  *string `json:"audio_path"`
	Status     string  `json:"status"`
	Text       string  `json:"text"`
}

type JobDetailResponse struct {
	JobID       string              `json:"job_id"`
	Engine      string              `json:"engine"`
	Voice       string              `json:"voice"`
	Speed       float64             `json:"speed"`
	TotalChunks int                 `json:"total_chunks"`
	Text        string              `json:"text"`
	Chunks      []ChunkItemResponse `json:"chunks"`
}

func (h *HistoryHandler) GetJobDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobID := chi.URLParam(r, "job_id")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	var job models.TTSJob
	if err := db.DB.Preload("Chunks").Where("id = ?", jobID).First(&job).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Job not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi CSDL"})
		return
	}

	if job.UserID != user.ID && user.Role != "admin" {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Forbidden"})
		return
	}

	bestChunks := make(map[int]models.TTSChunk)
	statusPriority := map[string]int{
		"done":       4,
		"processing": 3,
		"pending":    2,
		"error":      1,
	}

	for _, c := range job.Chunks {
		existing, found := bestChunks[c.ChunkIndex]
		if !found || statusPriority[c.Status] > statusPriority[existing.Status] {
			bestChunks[c.ChunkIndex] = c
		}
	}

	sortedChunks := make([]models.TTSChunk, 0, len(bestChunks))
	for _, c := range bestChunks {
		sortedChunks = append(sortedChunks, c)
	}

	sort.Slice(sortedChunks, func(i, j int) bool {
		return sortedChunks[i].ChunkIndex < sortedChunks[j].ChunkIndex
	})

	chunkResponses := make([]ChunkItemResponse, len(sortedChunks))
	chunkTexts := make([]string, 0, len(sortedChunks))

	for i, c := range sortedChunks {
		chunkResponses[i] = ChunkItemResponse{
			TaskID:     c.ID,
			ChunkIndex: c.ChunkIndex,
			AudioPath:  c.AudioPath,
			Status:     c.Status,
			Text:       c.Text,
		}
		if c.Text != "" {
			chunkTexts = append(chunkTexts, c.Text)
		}
	}

	fullText := job.Text
	if fullText == "" {
		fullText = strings.Join(chunkTexts, " ")
	}

	totalChunks := job.TotalChunks
	if len(sortedChunks) > totalChunks {
		totalChunks = len(sortedChunks)
	}

	_ = json.NewEncoder(w).Encode(JobDetailResponse{
		JobID:       job.ID,
		Engine:      job.Engine,
		Voice:       job.Voice,
		Speed:       job.Speed,
		TotalChunks: totalChunks,
		Text:        fullText,
		Chunks:      chunkResponses,
	})
}

type JobInitRequest struct {
	JobID       string  `json:"job_id"`
	Engine      string  `json:"engine"`
	Voice       string  `json:"voice"`
	Speed       float64 `json:"speed"`
	TotalChunks int     `json:"total_chunks"`
	Text        string  `json:"text"`
}

func (h *HistoryHandler) InitJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	var req JobInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.JobID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Dữ liệu không hợp lệ"})
		return
	}

	var job models.TTSJob
	err := db.DB.Where("id = ?", req.JobID).First(&job).Error
	if err == gorm.ErrRecordNotFound {
		job = models.TTSJob{
			ID:          req.JobID,
			UserID:      user.ID,
			Engine:      req.Engine,
			Voice:       req.Voice,
			Speed:       req.Speed,
			TotalChunks: req.TotalChunks,
			Text:        req.Text,
		}
		_ = db.DB.Create(&job).Error
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Job initialized successfully"})
}
