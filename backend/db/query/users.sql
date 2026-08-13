-- name: CreateUser :one
INSERT INTO users (id, username, password_hash, role, is_approved)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE username = $1 LIMIT 1;

-- ListUsers fetches one page of users, newest first.
--
-- Has a LIMIT for the same reason as ListUserHistorySummaries: the query previously returned every
-- row in the users table, so a deployment with many users caused every admin page open to load the
-- entire table. This is the only remaining query without a ceiling.
-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at DESC
LIMIT $1;

-- name: ApproveUser :one
UPDATE users
SET is_approved = true
WHERE id = $1
RETURNING *;
