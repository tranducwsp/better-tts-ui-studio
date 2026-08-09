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
-- Chuyển chunk còn đang chờ/xử lý sang 'cancelled'. Guard bằng status để cancel chạy đua với
-- worker hoàn tất không đè được kết quả: chunk đã 'done'/'error' giữ nguyên trạng thái.
UPDATE tts_chunks
SET status = 'cancelled',
    error_msg = COALESCE(error_msg, 'Đã huỷ theo yêu cầu'),
    updated_at = now()
WHERE id = $1
  AND status IN ('pending', 'processing');

-- name: ReconcileStaleChunks :execrows
-- Chuyển chunk mồ côi về 'error'. Một chunk đứng mãi ở pending/processing nghĩa là không ai
-- đang xử lý nó nữa — người điều hành thấy nó kẹt vĩnh viễn sau khi hàng đợi mất (Redis restart:
-- stream non-persistent, job trong đó trôi thẳng, không gì phục hồi lại được).
--
-- updated_at là mốc tiến triển cuối (CreateTTSChunk đến 'processing' chính là cột mốc bắt đầu);
-- tham số $1 là mốc thời gian giới hạn: chunk chưa chuyển trạng thái kể từ trước mốc đó là kẹt.
-- Ngưỡng này do STALE_CHUNK_AFTER_MINUTES quyết định và phải lớn hơn thời gian tối đa một
-- chunk hợp lệ có thể chạy (từng chunk ≤ TTS_CLIENT_TIMEOUT_SECONDS + chờ queue).
UPDATE tts_chunks
SET status = 'error',
    error_msg = COALESCE(error_msg, 'Job mất sau khi Redis khởi động lại, không phục hồi được'),
    updated_at = now()
WHERE status IN ('pending', 'processing')
  AND updated_at < $1::timestamptz;
