-- name: CreateAuthSession :one
INSERT INTO auth_sessions (id, session_family_id, user_id, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAuthSession :one
SELECT * FROM auth_sessions
WHERE id = $1;

-- name: RotateAuthSession :execrows
-- Mark the old token as replaced by the new token in the same family (rotation).
-- Guard revoked_at IS NULL: if the session has already been revoked (logout, family revoke), do not overwrite replaced_by.
UPDATE auth_sessions
SET revoked_at = CURRENT_TIMESTAMP,
    replaced_by = $2
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeAuthSession :execrows
-- Logout: revoke the current token. Do not set replaced_by, so every other token in the same family
-- (already revoked+replaced or this token itself) has no representative left for the session — the session is dead.
UPDATE auth_sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeAuthSessionFamily :execrows
-- Revoke all living tokens in the family. Used when an old token reuse is detected (stolen token):
-- the family is taken down, and the user must log in again.
UPDATE auth_sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE session_family_id = $1
  AND revoked_at IS NULL;

-- name: DeleteExpiredAuthSessions :execrows
-- Clean up expired sessions. Called opportunistically at login: small table, DELETE by index,
-- no need for a separate cleanup process.
DELETE FROM auth_sessions
WHERE expires_at < CURRENT_TIMESTAMP;
