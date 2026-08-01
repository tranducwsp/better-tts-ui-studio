-- 000008_add_pitch_emotion_to_tts_jobs.down.sql

ALTER TABLE tts_jobs DROP COLUMN IF EXISTS emotion;
ALTER TABLE tts_jobs DROP COLUMN IF EXISTS pitch;
