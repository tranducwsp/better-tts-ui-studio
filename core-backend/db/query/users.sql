-- name: CreateUser :one
INSERT INTO users (id, username, password_hash, role, is_approved)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE username = $1 LIMIT 1;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at DESC;

-- name: ApproveUser :one
UPDATE users
SET is_approved = true
WHERE id = $1
RETURNING *;
