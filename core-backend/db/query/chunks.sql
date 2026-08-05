-- name: CreateTTSChunk :one
INSERT INTO tts_chunks (id, job_id, chunk_index, text, audio_path, status, error_msg)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateTTSChunkStatus :one
UPDATE tts_chunks
SET status = $2,
    audio_path = COALESCE($3, audio_path),
    error_msg = COALESCE($4, error_msg)
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
