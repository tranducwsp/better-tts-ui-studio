package tests

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"backend/db"
	"backend/db/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbOrSkip nối tới PostgreSQL thật, hoặc bỏ qua bài.
//
// Ownership của job nằm ở tầng SQL (ON CONFLICT ... RETURNING user_id), nên không có cách kiểm
// nào trung thực mà không có cơ sở dữ liệu: một bản giả sẽ chỉ kiểm lại chính giả định của nó.
func dbOrSkip(t *testing.T) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("chưa đặt TEST_DATABASE_URL — bỏ qua bài tích hợp")
	}

	if db.Queries != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("không dựng được pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("không nối được PostgreSQL: %v", err)
	}

	db.Pool = pool
	db.Queries = sqlc.New(pool)
}

// makeUser tạo một người dùng dùng một lần cho bài kiểm thử.
func makeUser(t *testing.T, ctx context.Context) string {
	t.Helper()

	id := uuid.NewString()
	_, err := db.Queries.CreateUser(ctx, sqlc.CreateUserParams{
		ID:           id,
		Username:     "test-" + id[:8],
		PasswordHash: "x",
		Role:         "user",
		IsApproved:   true,
	})
	if err != nil {
		t.Fatalf("tạo người dùng: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	return id
}

// TestRegisterJobAndChunk_RejectsOtherUsersJob là nửa quan trọng nhất của phép kiểm quyền.
//
// job_id do client gửi lên và không có gì buộc nó là của người gọi. EnsureTTSJob là upsert, nên
// khi trùng khoá thì job của người khác vẫn nguyên — rồi chunk của người gọi được chèn vào dưới
// job đó. Hậu quả đo được: text của người gọi hiện ra trong lịch sử của người bị nhắm, và vì
// GetTaskOwner join qua tts_jobs.user_id nên chunk vừa chèn lại đọc thành của người kia.
func TestRegisterJobAndChunk_RejectsOtherUsersJob(t *testing.T) {
	dbOrSkip(t)

	ctx := context.Background()
	victim := makeUser(t, ctx)
	attacker := makeUser(t, ctx)

	jobID := uuid.NewString()
	victimTask := uuid.NewString()
	attackerTask := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM tts_chunks WHERE job_id = $1", jobID)
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM tts_jobs WHERE id = $1", jobID)
	})

	audio := db.JobAudioParams{Speed: 1.0}

	// Người bị nhắm tạo job của mình một cách bình thường.
	if err := db.RegisterJobAndChunk(ctx, victim, jobID, "standard", "v", audio, 1, victimTask, 0, "văn bản của tôi"); err != nil {
		t.Fatalf("người bị nhắm không tạo được job của chính mình: %v", err)
	}

	// Người tấn công gửi đúng job_id đó kèm text của mình.
	err := db.RegisterJobAndChunk(ctx, attacker, jobID, "standard", "v", audio, 1, attackerTask, 1, "văn bản chèn vào")
	if !errors.Is(err, db.ErrJobNotOwned) {
		t.Fatalf("chèn chunk vào job của người khác phải bị từ chối, nhận được %v", err)
	}

	// Và không được để lại dấu vết nào trong job của người bị nhắm.
	chunks, err := db.Queries.ListTTSChunksByJobID(ctx, jobID)
	if err != nil {
		t.Fatalf("đọc chunk: %v", err)
	}
	for _, c := range chunks {
		if c.ID == attackerTask {
			t.Error("chunk của người tấn công vẫn nằm trong job của người bị nhắm")
		}
		if c.Text == "văn bản chèn vào" {
			t.Error("text của người tấn công hiện ra trong lịch sử của người bị nhắm")
		}
	}
	if len(chunks) != 1 {
		t.Errorf("job của người bị nhắm có %d chunk, muốn 1", len(chunks))
	}
}

// TestRegisterJobAndChunk_AllowsOwnJob giữ chiều ngược lại: nhiều chunk của cùng một người vào
// cùng một job là đường đi bình thường của mọi lượt tổng hợp văn bản dài.
func TestRegisterJobAndChunk_AllowsOwnJob(t *testing.T) {
	dbOrSkip(t)

	ctx := context.Background()
	user := makeUser(t, ctx)

	jobID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM tts_chunks WHERE job_id = $1", jobID)
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM tts_jobs WHERE id = $1", jobID)
	})

	audio := db.JobAudioParams{Speed: 1.0}
	for i := range 3 {
		if err := db.RegisterJobAndChunk(ctx, user, jobID, "standard", "v", audio, 3, uuid.NewString(), i, "đoạn"); err != nil {
			t.Fatalf("chunk %d của chính chủ bị từ chối: %v", i, err)
		}
	}

	chunks, err := db.Queries.ListTTSChunksByJobID(ctx, jobID)
	if err != nil {
		t.Fatalf("đọc chunk: %v", err)
	}
	if len(chunks) != 3 {
		t.Errorf("có %d chunk, muốn 3", len(chunks))
	}
}
