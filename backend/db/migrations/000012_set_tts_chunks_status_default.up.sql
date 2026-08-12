-- Migration 000006 set DEFAULT 'processing' for tts_chunks.status, but a chunk should only transition
-- to processing when the worker actually begins — new rows are created in pending state.
-- Updating schema migration to align with schema.sql.

ALTER TABLE tts_chunks ALTER COLUMN status SET DEFAULT 'pending';