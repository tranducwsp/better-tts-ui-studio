-- 000008_add_pitch_emotion_to_tts_jobs.up.sql
-- Nullable on purpose: NULL means "the engine did not support this control for that
-- job", which is distinct from pitch = 0 (a valid, neutral pitch the user chose).

ALTER TABLE tts_jobs ADD COLUMN IF NOT EXISTS pitch DOUBLE PRECISION;
ALTER TABLE tts_jobs ADD COLUMN IF NOT EXISTS emotion VARCHAR(50);
