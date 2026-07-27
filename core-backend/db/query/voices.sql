-- name: CreateUserVoice :one
INSERT INTO user_voices (id, user_id, name, gender, region, style, file_path)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListUserVoices :many
SELECT * FROM user_voices
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: GetUserVoiceByID :one
SELECT * FROM user_voices
WHERE id = $1 AND user_id = $2 LIMIT 1;

-- name: DeleteUserVoice :exec
DELETE FROM user_voices
WHERE id = $1 AND user_id = $2;
