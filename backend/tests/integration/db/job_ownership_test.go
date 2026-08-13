package db_test

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

// dbOrSkip connects to a real PostgreSQL, or skips the test.
//
// Job ownership lives at the SQL layer (ON CONFLICT ... RETURNING user_id), so there is no
// honest way to test it without a database: a mock would only test its own assumptions.
func dbOrSkip(t *testing.T) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}

	if db.Queries != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("failed to create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("cannot connect to PostgreSQL: %v", err)
	}

	db.Pool = pool
	db.Queries = sqlc.New(pool)
}

// makeUser creates a one-shot user for the test.
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
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	return id
}

// TestRegisterJobAndChunk_RejectsOtherUsersJob is the most critical half of the ownership
// check.
//
// The job_id is sent by the client and nothing forces it to belong to the caller. EnsureTTSJob
// is an upsert, so when the key collides the other user's job remains intact — then the
// caller's chunk gets inserted under that job. The measurable consequence: the caller's text
// appears in the victim's history, and because GetTaskOwner joins via tts_jobs.user_id, the
// newly inserted chunk reads as belonging to the victim.
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

	// The victim creates their own job normally.
	if err := db.RegisterJobAndChunk(ctx, victim, jobID, "standard", "v", audio, 1, victimTask, 0, "my text"); err != nil {
		t.Fatalf("victim could not create their own job: %v", err)
	}

	// The attacker sends the same job_id with their own text.
	err := db.RegisterJobAndChunk(ctx, attacker, jobID, "standard", "v", audio, 1, attackerTask, 1, "injected text")
	if !errors.Is(err, db.ErrJobNotOwned) {
		t.Fatalf("inserting chunk into another user's job must be rejected, got %v", err)
	}

	// And must leave no trace in the victim's job.
	chunks, err := db.Queries.ListTTSChunksByJobID(ctx, jobID)
	if err != nil {
		t.Fatalf("read chunks: %v", err)
	}
	for _, c := range chunks {
		if c.ID == attackerTask {
			t.Error("attacker's chunk still in victim's job")
		}
		if c.Text == "injected text" {
			t.Error("attacker's text appears in victim's history")
		}
	}
	if len(chunks) != 1 {
		t.Errorf("victim's job has %d chunks, want 1", len(chunks))
	}
}

// TestRegisterJobAndChunk_AllowsOwnJob holds the opposite direction: multiple chunks from the
// same user into the same job is the normal path for every long-text synthesis.
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
		if err := db.RegisterJobAndChunk(ctx, user, jobID, "standard", "v", audio, 3, uuid.NewString(), i, "segment"); err != nil {
			t.Fatalf("own chunk %d rejected: %v", i, err)
		}
	}

	chunks, err := db.Queries.ListTTSChunksByJobID(ctx, jobID)
	if err != nil {
		t.Fatalf("read chunks: %v", err)
	}
	if len(chunks) != 3 {
		t.Errorf("got %d chunks, want 3", len(chunks))
	}
}
