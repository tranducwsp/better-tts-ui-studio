-- 000004_create_tts_chunks_table.up.sql

CREATE TABLE IF NOT EXISTS tts_chunks (
    id VARCHAR(64) PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL REFERENCES tts_jobs(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL DEFAULT 0,
    text TEXT NOT NULL DEFAULT '',
    audio_path VARCHAR(512),
    status VARCHAR(50) NOT NULL DEFAULT 'processing',
    error_msg TEXT
);
