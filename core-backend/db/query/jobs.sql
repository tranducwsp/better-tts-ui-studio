-- name: CreateTTSJob :one
INSERT INTO tts_jobs (id, user_id, engine, voice, speed, pitch, emotion, total_chunks, text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetTTSJobByID :one
SELECT * FROM tts_jobs
WHERE id = $1 LIMIT 1;

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
GROUP BY j.id
ORDER BY j.created_at DESC;

