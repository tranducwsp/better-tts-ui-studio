-- name: CreateTTSChunk :one
INSERT INTO tts_chunks (id, job_id, chunk_index, text, audio_path, status, error_msg)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateTTSChunkStatus :one
UPDATE tts_chunks
SET status = $2,
    audio_path = COALESCE($3, audio_path),
    error_msg = COALESCE($4, error_msg),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListTTSChunksByJobID :many
SELECT * FROM tts_chunks
WHERE job_id = $1
ORDER BY chunk_index ASC;

-- name: GetTaskOwner :one
SELECT j.user_id FROM tts_chunks c
JOIN tts_jobs j ON j.id = c.job_id
WHERE c.id = $1;

-- name: CancelTTSChunk :execrows
-- Transition a pending/processing chunk to 'cancelled'. Guard on status prevents cancel racing with
-- worker completion from overwriting the result: a 'done'/'error' chunk keeps its state.
UPDATE tts_chunks
SET status = 'cancelled',
    error_msg = COALESCE(error_msg, 'Cancelled by request'),
    updated_at = now()
WHERE id = $1
  AND status IN ('pending', 'processing');

-- name: ReconcileStaleChunks :execrows
-- Transition orphaned chunks to 'error'. A chunk stuck in pending/processing means no one is
-- processing it anymore — the operator sees it permanently stuck after the queue is lost (Redis restart:
-- non-persistent stream, jobs in it are gone, nothing can recover them).
--
-- updated_at is the last progress marker (CreateTTSChunk to 'processing' is the starting point);
-- parameter $1 is the cutoff timestamp: a chunk that has not transitioned since before that cutoff is stuck.
-- This threshold is determined by STALE_CHUNK_AFTER_MINUTES and must be greater than the maximum time
-- a legitimate chunk can run (each chunk ≤ TTS_CLIENT_TIMEOUT_SECONDS + queue wait).
UPDATE tts_chunks
SET status = 'error',
    error_msg = COALESCE(error_msg, 'Job lost after Redis restart, cannot be recovered'),
    updated_at = now()
WHERE status IN ('pending', 'processing')
  AND updated_at < $1::timestamptz;
