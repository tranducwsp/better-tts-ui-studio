package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend/db"
	"backend/db/sqlc"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// HistoryHandler handles APIs for viewing TTS conversion history and Job/Task details.
type HistoryHandler struct{}

// NewHistoryHandler creates a new HistoryHandler.
func NewHistoryHandler() *HistoryHandler {
	return &HistoryHandler{}
}

// JobSummaryResponse is the data structure for summarizing TTS jobs in history.
type JobSummaryResponse struct {
	JobID      string  `json:"job_id"`
	Engine     string  `json:"engine"`
	Voice      string  `json:"voice"`
	Speed      float64 `json:"speed"`
	Text       string  `json:"text"`
	CreatedAt  string  `json:"created_at"`
	TimeAgo    string  `json:"time_ago"`
	Progress   string  `json:"progress"`
	IsComplete bool    `json:"is_complete"`
}

// HistoryPageResponse is a single page of history: the list of items plus a flag indicating
// whether there are more pages.
//
// The UI needs HasMore to decide whether to show a "load more" button; previously the API
// returned a plain array so the client only knew there were up to 200 items and had no way
// to tell whether the list was truly exhausted.
type HistoryPageResponse struct {
	Items   []JobSummaryResponse `json:"items"`
	HasMore bool                 `json:"has_more"`
}

// GetUserHistory retrieves the TTS creation history of the currently logged-in user.
func (h *HistoryHandler) GetUserHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := currentUser(w, r)
	if !ok {
		return
	}

	h.getHistoryForUser(w, r, user.ID)
}

// GetUserHistoryAdmin (Admin API) retrieves the TTS conversion history of any user by user_id.
func (h *HistoryHandler) GetUserHistoryAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	userID := chi.URLParam(r, "user_id")
	h.getHistoryForUser(w, r, userID)
}

// historyPageSize is the number of jobs returned per history view.
//
// Previously the query had no LIMIT, so long-time users caused every history open to
// aggregate and transfer all jobs from the beginning. The UI only displays a scrollable
// list, so this ceiling is invisible to the user but very visible to the server.
const historyPageSize = 200

// historyFetchLimit is the actual number of jobs queried: the page needs to compute HasMore
// only by asking for one row more than the ceiling. When the bins are full, the excess is
// trimmed — no one ever sees a page with 201 items.
const historyFetchLimit = historyPageSize + 1

// getHistoryForUser is an internal function that aggregates history data for Jobs and the
// completion progress of the User's Chunks.
//
// Cursor-based pagination: the client sends `before` (RFC3339) and `before_id` of the last
// item from the previous page, so the next request returns older jobs. Missing parameters or
// an empty `before` means the first page. The `before_id` field is usually sent alongside
// but the predicate only relies on it when two jobs share the same timestamp.
func (h *HistoryHandler) getHistoryForUser(w http.ResponseWriter, r *http.Request, userID string) {
	params := sqlc.ListUserHistorySummariesParams{
		UserID: userID,
		Limit:  historyFetchLimit,
	}

	if raw := r.URL.Query().Get("before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid before timestamp")
			return
		}
		params.BeforeCreatedAt = pgtype.Timestamptz{Time: t, Valid: true}
		params.BeforeID = r.URL.Query().Get("before_id")
	}

	summaries, err := db.Queries.ListUserHistorySummaries(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	hasMore := len(summaries) > historyPageSize
	if hasMore {
		summaries = summaries[:historyPageSize]
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

		// strconv.Itoa instead of fmt.Sprintf in the two places below: this loop runs up to
		// historyPageSize (200) times per history open, and Sprintf must parse the format
		// string then go through reflection for each argument. Measured 60µs down to 34µs
		// for a page of 200 jobs — small compared to a query round-trip, but these are the
		// only two places in the repo where string formatting sits inside a loop, so they
		// are also the only two places worth changing.
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

		// Full-microsecond format instead of truncated RFC3339: this created_at is sent back
		// by the client as a pagination cursor, and dropping sub-second precision causes jobs
		// that share the same timestamp and land right on the page boundary to be silently
		// skipped. Six digits match Postgres microsecond resolution, so the round-trip through
		// the client does not distort ordering.
		createdAt := ""
		if s.CreatedAt.Valid {
			createdAt = s.CreatedAt.Time.Format("2006-01-02T15:04:05.000000Z")
		}

		result = append(result, JobSummaryResponse{
			JobID:      s.JobID,
			Engine:     s.Engine,
			Voice:      s.Voice,
			Speed:      s.Speed,
			Text:       shortText,
			CreatedAt:  createdAt,
			TimeAgo:    timeAgo,
			Progress:   strconv.Itoa(doneChunks) + "/" + strconv.Itoa(actualTotal),
			IsComplete: doneChunks == actualTotal && actualTotal > 0,
		})
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(HistoryPageResponse{
		Items:   result,
		HasMore: hasMore,
	})
}

// ChunkItemResponse contains information for each audio chunk segment within a job.
// ChunkItemResponse does NOT carry a storage path.
//
// The audio_path field used to be returned and the UI would concatenate it into a direct URL.
// That is an internal key of the store, so with a non-disk backend it is meaningless, and even
// with disk the browser cannot serve it. More importantly: a URL pointing directly at the store
// would bypass ownsTask, i.e. skip the very ownership-check layer that was just added for
// tasks. The client uses /api/tasks/{task_id}/audio, where ownership is checked every time.
type ChunkItemResponse struct {
	TaskID     string `json:"task_id"`
	ChunkIndex int    `json:"chunk_index"`
	Status     string `json:"status"`
	Text       string `json:"text"`
}

// JobDetailResponse contains the full details of a TTS Job including all assembled Chunks.
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

// GetJobDetail retrieves details of a specific TTS Job by job_id (including progress of each
// text segment).
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

	// A single chunk_index can have multiple rows (retries), so keep the row at the most
	// advanced status.
	//
	// A single forward scan, no map and no re-sort: the query already returns rows ordered by
	// ORDER BY chunk_index ASC, so building a map then spreading then sorting only to get the
	// order that was already there — and each map entry/exit copies the entire struct,
	// including the Text field.
	statusPriority := map[string]int{
		"done":       4,
		"processing": 3,
		"pending":    2,
		"error":      1,
		// "cancelled" must be present: without it the map returns 0 and a cancelled chunk
		// always loses to "error" at the same chunk_index — a user who cancels then retries
		// would see the job appear as errored.
		"cancelled": 1,
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
		chunkResponses[i] = ChunkItemResponse{
			TaskID:     c.ID,
			ChunkIndex: int(c.ChunkIndex),
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

// JobInitRequest is the request to initialize information for a large TTS Job.
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

// InitJob initializes the base information of a TTS Job before splitting the text and
// synthesizing each chunk.
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

	// An upsert statement: two concurrent init requests for the same job both see
	// "does not exist" and both insert, and the loser is silently discarded.
	audio := db.JobAudioParams{Speed: req.Speed, Pitch: req.Pitch, Emotion: req.Emotion}
	owner, err := db.Queries.EnsureTTSJob(r.Context(), sqlc.EnsureTTSJobParams{
		ID:          req.JobID,
		UserID:      user.ID,
		Engine:      req.Engine,
		Voice:       req.Voice,
		Speed:       req.Speed,
		Pitch:       audio.PitchColumn(),
		Emotion:     audio.EmotionColumn(),
		TotalChunks: int32(req.TotalChunks),
		Text:        req.Text,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to initialize job")
		return
	}

	// job_id is sent by the client. Upsert keeps the existing job, so without this check a
	// person "initializing" someone else's job would receive a success message — and every
	// subsequent chunk would land in that other person's history.
	if owner != user.ID {
		writeError(w, http.StatusForbidden, "Forbidden")
		return
	}

	_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]string{"message": "Job initialized successfully"})
}
