-- PostgreSQL DDL Schema for VieNeu Core Backend

CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(64) PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    is_approved BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_voices (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    model_id VARCHAR(50) NOT NULL DEFAULT 'clone',
    name VARCHAR(255) NOT NULL,
    file_path VARCHAR(512) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tts_jobs (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    engine VARCHAR(50) NOT NULL,
    voice VARCHAR(255) NOT NULL,
    speed DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    pitch DOUBLE PRECISION,
    emotion VARCHAR(50),
    total_chunks INT NOT NULL DEFAULT 1,
    text TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tts_chunks (
    id VARCHAR(64) PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL REFERENCES tts_jobs(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL DEFAULT 0,
    text TEXT NOT NULL DEFAULT '',
    audio_path VARCHAR(512),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    error_msg TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Foreign keys are not automatically indexed in Postgres. History queries filter by user_id
-- and sort by created_at, joining chunks on job_id; without these indexes every history
-- lookup results in a full table scan.
CREATE INDEX IF NOT EXISTS idx_tts_chunks_job_id ON tts_chunks (job_id);
CREATE INDEX IF NOT EXISTS idx_tts_jobs_user_created ON tts_jobs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_voices_user_model ON user_voices (user_id, model_id);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id                VARCHAR(64) PRIMARY KEY,
    session_family_id VARCHAR(64) NOT NULL,
    user_id           VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at        TIMESTAMPTZ NOT NULL,
    revoked_at        TIMESTAMPTZ,
    replaced_by       VARCHAR(64),
    user_agent        TEXT,
    ip                VARCHAR(64)
);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_user   ON auth_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_family ON auth_sessions (session_family_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires ON auth_sessions (expires_at);
