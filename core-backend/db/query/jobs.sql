-- name: CreateTTSJob :one
INSERT INTO tts_jobs (id, user_id, engine, voice, speed, total_chunks, text)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetTTSJobByID :one
SELECT * FROM tts_jobs
WHERE id = $1 LIMIT 1;

-- name: GetTTSJobByIDAndUser :one
SELECT * FROM tts_jobs
WHERE id = $1 AND user_id = $2 LIMIT 1;

-- name: ListTTSJobsByUserID :many
SELECT * FROM tts_jobs
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListAllTTSJobs :many
SELECT * FROM tts_jobs
ORDER BY created_at DESC;
