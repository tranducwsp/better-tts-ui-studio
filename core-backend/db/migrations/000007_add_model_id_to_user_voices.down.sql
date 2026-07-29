DROP INDEX IF EXISTS idx_user_voices_user_model;
ALTER TABLE user_voices DROP COLUMN IF NOT EXISTS model_id;
