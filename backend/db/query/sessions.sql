-- name: CreateAuthSession :one
INSERT INTO auth_sessions (id, session_family_id, user_id, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAuthSession :one
SELECT * FROM auth_sessions
WHERE id = $1;

-- name: RotateAuthSession :execrows
-- Đánh dấu token cũ đã bị thay thế bởi token mới trong cùng family (rotation).
-- Guard revoked_at IS NULL: nếu session đã bị revoke (logout, family revoke), không ghi đè replaced_by.
UPDATE auth_sessions
SET revoked_at = CURRENT_TIMESTAMP,
    replaced_by = $2
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeAuthSession :execrows
-- Logout: thu hồi token đang dùng. Không đặt replaced_by, nên mọi token khác cùng family
-- (vốn đã revoked+replaced hoặc là token này) đều không còn ai đại diện cho phiên — phiên chết.
UPDATE auth_sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeAuthSessionFamily :execrows
-- Thu hồi mọi token còn sống trong family. Dùng khi phát hiện reuse token cũ (bị đánh cắp):
-- family bị hạ, người dùng phải đăng nhập lại.
UPDATE auth_sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE session_family_id = $1
  AND revoked_at IS NULL;

-- name: DeleteExpiredAuthSessions :execrows
-- Dọn các phiên đã hết hạn. Gọi opportunistic ngay tại login: bảng nhỏ, DELETE theo index,
-- không cần một tiến trình dọn riêng.
DELETE FROM auth_sessions
WHERE expires_at < CURRENT_TIMESTAMP;
