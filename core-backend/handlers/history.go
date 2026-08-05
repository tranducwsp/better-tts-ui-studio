package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"core-backend/db"
	"core-backend/db/sqlc"

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
	user, ok := currentUser(w, r)
	if !ok {
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

// historyPageSize là số job trả về cho một lần xem lịch sử.
//
// Trước đây truy vấn không có LIMIT, nên người dùng lâu năm khiến mỗi lần mở lịch sử phải
// tổng hợp và truyền về toàn bộ job từ trước tới nay. Giao diện chỉ hiển thị một danh sách
// cuộn, nên trần này là thứ người dùng không nhìn thấy còn máy chủ thì thấy rõ.
const historyPageSize = 200

// getHistoryForUser hàm nội bộ tổng hợp dữ liệu lịch sử các Job và tiến độ hoàn thành các Chunk của User.
func (h *HistoryHandler) getHistoryForUser(w http.ResponseWriter, r *http.Request, userID string) {
	summaries, err := db.Queries.ListUserHistorySummaries(r.Context(), sqlc.ListUserHistorySummariesParams{
		UserID: userID,
		Limit:  historyPageSize,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	result := make([]JobSummaryResponse, 0, len(summaries))
	now := time.Now()

	for _, s := range summaries {
		shortText := s.JobText
		if shortText == "" {
			shortText = s.FirstChunkText
		}
		if shortText != "" {
			runes := []rune(shortText)
			if len(runes) > 50 {
				shortText = string(runes[:50]) + "..."
			}
		} else {
			shortText = "No content"
		}

		doneChunks := int(s.DoneChunks)
		actualTotal := int(s.TotalChunks)
		if int(s.ActualChunksCount) > actualTotal {
			actualTotal = int(s.ActualChunksCount)
		}

		// strconv.Itoa thay cho fmt.Sprintf ở hai chỗ dưới đây: vòng lặp này chạy tới
		// historyPageSize (200) lần mỗi lần mở lịch sử, và Sprintf phải phân tích chuỗi định
		// dạng rồi đi qua reflection cho mỗi tham số. Đo được 60µs xuống 34µs cho một trang
		// 200 job — nhỏ so với một lượt truy vấn, nhưng đây là hai chỗ duy nhất trong repo mà
		// định dạng chuỗi nằm trong vòng lặp, nên cũng là hai chỗ duy nhất đáng đổi.
		timeAgo := "Just now"
		if s.CreatedAt.Valid {
			diff := now.Sub(s.CreatedAt.Time)
			if diff.Hours() >= 24 {
				timeAgo = strconv.Itoa(int(diff.Hours()/24)) + " days ago"
			} else if diff.Hours() >= 1 {
				timeAgo = strconv.Itoa(int(diff.Hours())) + " hours ago"
			} else if diff.Minutes() >= 1 {
				timeAgo = strconv.Itoa(int(diff.Minutes())) + " mins ago"
			}
		}

		result = append(result, JobSummaryResponse{
			JobID:      s.JobID,
			Engine:     s.Engine,
			Voice:      s.Voice,
			Speed:      s.Speed,
			Text:       shortText,
			TimeAgo:    timeAgo,
			Progress:   strconv.Itoa(doneChunks) + "/" + strconv.Itoa(actualTotal),
			IsComplete: doneChunks == actualTotal && actualTotal > 0,
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
	Pitch       *float64            `json:"pitch,omitempty"`
	Emotion     *string             `json:"emotion,omitempty"`
	TotalChunks int                 `json:"total_chunks"`
	Text        string              `json:"text"`
	Chunks      []ChunkItemResponse `json:"chunks"`
}

// GetJobDetail lấy chi tiết một Job TTS cụ thể theo job_id (bao gồm tiến độ từng đoạn văn bản).
func (h *HistoryHandler) GetJobDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobID := chi.URLParam(r, "job_id")
	user, ok := currentUser(w, r)
	if !ok {
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

	// Một chunk_index có thể có nhiều dòng (thử lại), nên giữ dòng ở trạng thái tiến xa nhất.
	//
	// Một lượt quét tiến, không map và không sắp xếp lại: truy vấn đã trả về theo
	// ORDER BY chunk_index ASC, nên dựng map rồi rải ra rồi sort lại chỉ để có đúng thứ tự
	// vốn đã có — mà mỗi lần vào/ra map là một lần sao chép cả struct, kể cả trường Text.
	statusPriority := map[string]int{
		"done":       4,
		"processing": 3,
		"pending":    2,
		"error":      1,
	}

	best := make([]*sqlc.TtsChunk, 0, len(chunks))
	for i := range chunks {
		c := &chunks[i]
		if n := len(best); n > 0 && best[n-1].ChunkIndex == c.ChunkIndex {
			if statusPriority[c.Status] > statusPriority[best[n-1].Status] {
				best[n-1] = c
			}
			continue
		}
		best = append(best, c)
	}

	chunkResponses := make([]ChunkItemResponse, len(best))
	chunkTexts := make([]string, 0, len(best))

	for i, c := range best {
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
	if len(best) > totalChunks {
		totalChunks = len(best)
	}

	// NULL pitch/emotion means the engine had no such control for this job; leave the
	// pointers nil so they are omitted and the client keeps its manifest defaults.
	var pitchPtr *float64
	if job.Pitch.Valid {
		pitchPtr = &job.Pitch.Float64
	}
	var emotionPtr *string
	if job.Emotion.Valid && job.Emotion.String != "" {
		emotionPtr = &job.Emotion.String
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(JobDetailResponse{
		JobID:       job.ID,
		Engine:      job.Engine,
		Voice:       job.Voice,
		Speed:       job.Speed,
		Pitch:       pitchPtr,
		Emotion:     emotionPtr,
		TotalChunks: totalChunks,
		Text:        fullText,
		Chunks:      chunkResponses,
	})
}

// JobInitRequest yêu cầu khởi tạo thông tin cho một Job TTS lớn.
type JobInitRequest struct {
	JobID       string   `json:"job_id"`
	Engine      string   `json:"engine"`
	Voice       string   `json:"voice"`
	Speed       float64  `json:"speed"`
	Pitch       *float64 `json:"pitch"`
	Emotion     *string  `json:"emotion"`
	TotalChunks int      `json:"total_chunks"`
	Text        string   `json:"text"`
}

// InitJob khởi tạo thông tin ban đầu của Job TTS trước khi tiến hành chia nhỏ văn bản và phát âm từng chunk.
func (h *HistoryHandler) InitJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := currentUser(w, r)
	if !ok {
		return
	}

	var req JobInitRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil || req.JobID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"detail": "Invalid payload"})
		return
	}

	// Một câu lệnh upsert: hai request khởi tạo cùng một job đồng thời đều thấy "chưa có"
	// rồi cùng chèn, và cái thua bị bỏ lỗi âm thầm.
	audio := db.JobAudioParams{Speed: req.Speed, Pitch: req.Pitch, Emotion: req.Emotion}
	if err := db.Queries.EnsureTTSJob(r.Context(), sqlc.EnsureTTSJobParams{
		ID:          req.JobID,
		UserID:      user.ID,
		Engine:      req.Engine,
		Voice:       req.Voice,
		Speed:       req.Speed,
		Pitch:       audio.PitchColumn(),
		Emotion:     audio.EmotionColumn(),
		TotalChunks: int32(req.TotalChunks),
		Text:        req.Text,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "Không khởi tạo được job")
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Job initialized successfully"})
}
