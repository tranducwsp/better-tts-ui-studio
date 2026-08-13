-- 000002_create_user_voices_table.up.sql

CREATE TABLE IF NOT EXISTS user_voices (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    gender VARCHAR(50),
    region VARCHAR(50),
    style VARCHAR(50),
    file_path VARCHAR(512) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
