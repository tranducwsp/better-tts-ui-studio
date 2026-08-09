ALTER TABLE user_voices ADD COLUMN IF NOT EXISTS model_id VARCHAR(50) NOT NULL DEFAULT 'clone';
CREATE INDEX IF NOT EXISTS idx_user_voices_user_model ON user_voices (user_id, model_id);
