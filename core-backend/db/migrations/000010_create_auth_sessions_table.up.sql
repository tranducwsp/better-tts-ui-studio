-- 000010_create_auth_sessions_table.up.sql

-- Sổ đăng ký refresh token: mỗi dòng = MỘT refresh token đang được phát hành, thuộc về một
-- phiên đăng nhập (session_family_id). Trước đây refresh token là JWT stateless sống 30 ngày,
-- không có cách nào thu hồi — logout chỉ xoá cookie, token vẫn hợp lệ phía server.
--
-- Rotation: mỗi lượt refresh tạo dòng MỚI (jti mới) cùng family, và đánh dấu dòng cũ
-- revoked_at + replaced_by. Một family chỉ có đúng một dòng còn sống tại mỗi thời điểm.
--
-- Replay: token cũ (revoked + replaced) được trình lên → bị từ chối. Với trình duyệt mở nhiều
-- tab, cookie refresh được chia sẻ chung một jar nên lượt retry tự dùng token mới nhất — không
-- cần tự sát hại family khi gặp race.
CREATE TABLE IF NOT EXISTS auth_sessions (
    id                VARCHAR(64) PRIMARY KEY,  -- jti của refresh JWT
    session_family_id VARCHAR(64) NOT NULL,     -- 1 login = 1 family; rotation giữ nguyên
    user_id           VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at        TIMESTAMPTZ NOT NULL,     -- hạn TUYỆT ĐỐI, không bị rotation kéo dài
    revoked_at        TIMESTAMPTZ,              -- NULL = còn sống
    replaced_by       VARCHAR(64),              -- jti của token thay thế (rotation)
    user_agent        TEXT,
    ip                VARCHAR(64)
);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_user   ON auth_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_family ON auth_sessions (session_family_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires ON auth_sessions (expires_at);
