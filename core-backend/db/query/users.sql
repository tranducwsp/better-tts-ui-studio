-- name: CreateUser :one
INSERT INTO users (id, username, password_hash, role, is_approved)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE username = $1 LIMIT 1;

-- ListUsers lấy một trang danh sách người dùng, mới nhất trước.
--
-- Có LIMIT vì cùng lý do với ListUserHistorySummaries: trước đây truy vấn trả về mọi hàng
-- trong bảng users, nên một triển khai đông người dùng khiến mỗi lần mở trang quản trị phải
-- tải toàn bộ bảng về. Đây là truy vấn duy nhất còn sót lại không có trần.
-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at DESC
LIMIT $1;

-- name: ApproveUser :one
UPDATE users
SET is_approved = true
WHERE id = $1
RETURNING *;
