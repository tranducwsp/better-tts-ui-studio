-- name: CreateTTSJob :one
INSERT INTO tts_jobs (id, user_id, engine, voice, speed, pitch, emotion, total_chunks, text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- EnsureTTSJob tạo job nếu chưa có, và trả về chủ sở hữu thật của nó.
--
-- Thay cho cặp GetTTSJobByID-rồi-CreateTTSJob: hai chunk đầu tiên của cùng một job tới song
-- song đều thấy "chưa tồn tại" rồi cùng chèn, và cái thua bị bỏ lỗi âm thầm. Một câu lệnh
-- vừa hết đua vừa bớt một lượt đi lại tới cơ sở dữ liệu trên đường đi của mỗi chunk.
--
-- DO UPDATE ... id = tts_jobs.id chứ không DO NOTHING: job_id do client gửi lên, nên người
-- gọi phải biết được job đã tồn tại là của ai. DO NOTHING không trả dòng nào khi trùng, nên
-- không phân biệt được "vừa tạo" với "đã có của người khác" — và người gọi chèn chunk của
-- mình vào job của người khác mà không hay. Ghi giả một trường để RETURNING luôn có dòng.
-- name: EnsureTTSJob :one
INSERT INTO tts_jobs (id, user_id, engine, voice, speed, pitch, emotion, total_chunks, text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET id = tts_jobs.id
RETURNING user_id;

-- name: GetTTSJobByID :one
SELECT * FROM tts_jobs
WHERE id = $1 LIMIT 1;

-- ListUserHistorySummaries lấy một trang lịch sử, mới nhất trước.
--
-- Có LIMIT vì trước đây truy vấn trả về mọi job của người dùng: ai dùng nhiều thì mỗi lần
-- mở lịch sử là tổng hợp rồi truyền về hàng nghìn dòng mà giao diện chỉ hiển thị một phần.
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
ORDER BY j.created_at DESC
LIMIT $2;

