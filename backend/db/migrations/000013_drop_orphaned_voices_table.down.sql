-- Khôi phục lại bảng voices nếu cần quay lại. Schema khớp với migration 000003 gốc.
CREATE TABLE IF NOT EXISTS voices (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    audio_path VARCHAR(512) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
