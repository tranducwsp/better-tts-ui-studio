-- 000011_add_updated_at_to_tts_chunks.up.sql

-- Tracks the last progress timestamp of a chunk. Previously the table had no timestamp column,
-- making it impossible to determine if a chunk was still active or stuck — an orphaned chunk due to
-- lost queue messages (e.g. Redis restart) would stay in 'processing' forever. updated_at serves as
-- an anchor for cron reconciliation to confirm stale chunks.
ALTER TABLE tts_chunks ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;