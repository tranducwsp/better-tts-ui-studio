package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"core-backend/db"
	"core-backend/db/sqlc"
	"core-backend/middleware"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
)

// HistoryHandler xử lý các API xem lịch sử chuyển đổi TTS và chi tiết các Job/Task.
type HistoryHandler struct{}

// NewHistoryHandler khởi tạo HistoryHandler.
func NewHistoryHandler() *HistoryHandler {
	return &HistoryHandler{}
}

// JobSummaryResponse cấu trúc dữ liệu tóm tắt công việc TTS trong lịch sử.
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

// GetUserHistory lấy danh sách lịch sử tạo TTS của chính người dùng đang đăng nhập.
func (h *HistoryHandler) GetUserHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	h.getHistoryForUser(w, r, user.ID)
}

// GetUserHistoryAdmin (Admin API) lấy lịch sử chuyển đổi TTS của một người dùng bất kỳ theo user_id.
func (h *HistoryHandler) GetUserHistoryAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := chi.URLParam(r, "user_id")
	h.getHistoryForUser(w, r, userID)
}

// getHistoryForUser hàm nội bộ tổng hợp dữ liệu lịch sử các Job và tiến độ hoàn thành các Chunk của User.
func (h *HistoryHandler) getHistoryForUser(w http.ResponseWriter, r *http.Request, userID string) {
	jobs, err := db.Queries.ListTTSJobsByUserID(r.Context(), userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Lỗi CSDL"})
		return
	}

	result := make([]JobSummaryResponse, 0, len(jobs))
	now := time.Now()

	for _, job := range jobs {
		chunks, _ := db.Queries.ListTTSChunksByJobID(r.Context(), job.ID)
		shortText := job.Text
		if shortText != "" {
			runes := []rune(shortText)
			if len(runes) > 50 {
				shortText = string(runes[:50]) + "..."
			}
		} else {
			var firstChunk *sqlc.TtsChunk
			for i := range chunks {
				if chunks[i].ChunkIndex == 0 {
					firstChunk = &chunks[i]
					break
				}
			}
			if firstChunk == nil && len(chunks) > 0 {
				firstChunk = &chunks[0]
			}
			if firstChunk != nil {
				shortText = firstChunk.Text
			} else {
				shortText = "Chưa có nội dung"
			}
		}

		uniqueIndexes := make(map[int32]bool)
		doneIndexes := make(map[int32]bool)
		for _, c := range chunks {
			uniqueIndexes[c.ChunkIndex] = true
			if c.Status == "done" {
				doneIndexes[c.ChunkIndex] = true
			}
		}

		doneChunks := len(doneIndexes)
		actualTotal := int(job.TotalChunks)
		if len(uniqueIndexes) > actualTotal {
			actualTotal = len(uniqueIndexes)
		}

		timeAgo := "Vừa xong"
		if job.CreatedAt.Valid {
			diff := now.Sub(job.CreatedAt.Time)
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

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(result)
}

// ChunkItemResponse thông tin từng đoạn audio chunk trong job.
type ChunkItemResponse struct {
	TaskID     string  `json:"task_id"`
	ChunkIndex int     `json:"chunk_index"`
	AudioPath  *string `json:"audio_path"`
	Status     string  `json:"status"`
	Text       string  `json:"text"`
}

// JobDetailResponse chi tiết đầy đủ của một Job TTS bao gồm tất cả các đoạn Chunks ghép lại.
type JobDetailResponse struct {
	JobID       string              `json:"job_id"`
	Engine      string              `json:"engine"`
	Voice       string              `json:"voice"`
	Speed       float64             `json:"speed"`
	TotalChunks int                 `json:"total_chunks"`
	Text        string              `json:"text"`
	Chunks      []ChunkItemResponse `json:"chunks"`
}

// GetJobDetail lấy chi tiết một Job TTS cụ thể theo job_id (bao gồm tiến độ từng đoạn văn bản).
func (h *HistoryHandler) GetJobDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobID := chi.URLParam(r, "job_id")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	job, err := db.Queries.GetTTSJobByID(r.Context(), jobID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Job not found"})
		return
	}

	if job.UserID != user.ID && user.Role != "admin" {
		w.WriteHeader(http.StatusForbidden)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Forbidden"})
		return
	}

	chunks, _ := db.Queries.ListTTSChunksByJobID(r.Context(), job.ID)
	bestChunks := make(map[int32]sqlc.TtsChunk)
	statusPriority := map[string]int{
		"done":       4,
		"processing": 3,
		"pending":    2,
		"error":      1,
	}

	for _, c := range chunks {
		existing, found := bestChunks[c.ChunkIndex]
		if !found || statusPriority[c.Status] > statusPriority[existing.Status] {
			bestChunks[c.ChunkIndex] = c
		}
	}

	sortedChunks := make([]sqlc.TtsChunk, 0, len(bestChunks))
	for _, c := range bestChunks {
		sortedChunks = append(sortedChunks, c)
	}

	sort.Slice(sortedChunks, func(i, j int) bool {
		return sortedChunks[i].ChunkIndex < sortedChunks[j].ChunkIndex
	})

	chunkResponses := make([]ChunkItemResponse, len(sortedChunks))
	chunkTexts := make([]string, 0, len(sortedChunks))

	for i, c := range sortedChunks {
		var audioPathPtr *string
		if c.AudioPath.Valid {
			audioPathPtr = &c.AudioPath.String
		}
		chunkResponses[i] = ChunkItemResponse{
			TaskID:     c.ID,
			ChunkIndex: int(c.ChunkIndex),
			AudioPath:  audioPathPtr,
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

	totalChunks := int(job.TotalChunks)
	if len(sortedChunks) > totalChunks {
		totalChunks = len(sortedChunks)
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(JobDetailResponse{
		JobID:       job.ID,
		Engine:      job.Engine,
		Voice:       job.Voice,
		Speed:       job.Speed,
		TotalChunks: totalChunks,
		Text:        fullText,
		Chunks:      chunkResponses,
	})
}

// JobInitRequest yêu cầu khởi tạo thông tin cho một Job TTS lớn.
type JobInitRequest struct {
	JobID       string  `json:"job_id"`
	Engine      string  `json:"engine"`
	Voice       string  `json:"voice"`
	Speed       float64 `json:"speed"`
	TotalChunks int     `json:"total_chunks"`
	Text        string  `json:"text"`
}

// InitJob khởi tạo thông tin ban đầu của Job TTS trước khi tiến hành chia nhỏ văn bản và phát âm từng chunk.
func (h *HistoryHandler) InitJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := middleware.GetCurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Not authenticated"})
		return
	}

	var req JobInitRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil || req.JobID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Dữ liệu không hợp lệ"})
		return
	}

	_, err := db.Queries.GetTTSJobByID(r.Context(), req.JobID)
	if err != nil {
		_, _ = db.Queries.CreateTTSJob(r.Context(), sqlc.CreateTTSJobParams{
			ID:          req.JobID,
			UserID:      user.ID,
			Engine:      req.Engine,
			Voice:       req.Voice,
			Speed:       req.Speed,
			TotalChunks: int32(req.TotalChunks),
			Text:        req.Text,
		})
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Job initialized successfully"})
}
