-- Restore original default from migration 000006 for step-by-step rollback.

ALTER TABLE tts_chunks ALTER COLUMN status SET DEFAULT 'processing';