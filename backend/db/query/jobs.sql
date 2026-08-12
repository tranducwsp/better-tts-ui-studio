-- name: CreateTTSJob :one
INSERT INTO tts_jobs (id, user_id, engine, voice, speed, pitch, emotion, total_chunks, text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- EnsureTTSJob creates a job if it does not exist yet, and returns the true owner.
--
-- Replaces the GetTTSJobByID-then-CreateTTSJob pair: the first two chunks of the same job arriving
-- concurrently both see "not exists" and both try to insert, and the loser silently drops the error.
-- A single statement both races and saves a round-trip to the database on each chunk's path.
--
-- DO UPDATE ... id = tts_jobs.id rather than DO NOTHING: job_id is sent by the client, so the caller
-- must know whether an existing job belongs to them. DO NOTHING returns no row on conflict, so it
-- cannot distinguish "just created" from "already owned by someone else" — and the caller would insert
-- its chunk into someone else's job unknowingly. A dummy field write ensures RETURNING always has a row.
-- name: EnsureTTSJob :one
INSERT INTO tts_jobs (id, user_id, engine, voice, speed, pitch, emotion, total_chunks, text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET id = tts_jobs.id
RETURNING user_id;

-- name: GetTTSJobByID :one
SELECT * FROM tts_jobs
WHERE id = $1 LIMIT 1;

-- ListUserHistorySummaries fetches one page of history, newest first.
--
-- Has a LIMIT because the query previously returned every job for a user: heavy users would cause
-- each history page open to aggregate and transmit thousands of rows while the UI only shows a subset.
--
-- Keyset pagination instead of OFFSET: the next page passes the `before_id` of the last item on the
-- previous page along with its `created_at`. The comparison condition (created_at, id) follows the
-- ORDER BY order so the planner can use the index instead of scanning excess rows; OFFSET requires
-- re-skipping more rows the deeper you page. `before_created_at` accepts NULL for the "first page"
-- — the clause falls into the first branch then and filters nothing. The next page is only valid
-- when both columns are provided (before_id is a tiebreaker for duplicate timestamps).
--
-- GROUP BY j.id: multiple chunks belong to the same job when workers split them — flattened into
-- one summary row.
-- name: ListUserHistorySummaries :many
SELECT
    j.id AS job_id,
    j.engine,
    j.voice,
    j.speed,
    j.total_chunks,
    j.text AS job_text,
    j.created_at,
    COALESCE(COUNT(DISTINCT CASE WHEN c.status = 'done' THEN c.chunk_index END), 0)::int AS done_chunks,
    COALESCE(COUNT(DISTINCT c.chunk_index), 0)::int AS actual_chunks_count,
    COALESCE(MIN(c.text), '')::text AS first_chunk_text
FROM tts_jobs j
LEFT JOIN tts_chunks c ON j.id = c.job_id
WHERE j.user_id = $1
  AND (sqlc.arg(before_created_at)::timestamptz IS NULL
       OR j.created_at < sqlc.arg(before_created_at)::timestamptz
       OR (j.created_at = sqlc.arg(before_created_at)::timestamptz AND j.id < sqlc.arg(before_id)::text))
GROUP BY j.id
ORDER BY j.created_at DESC, j.id DESC
LIMIT $2;

