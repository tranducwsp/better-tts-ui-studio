-- 000011_add_updated_at_to_tts_chunks.down.sql

ALTER TABLE tts_chunks DROP COLUMN IF EXISTS updated_at;