-- The legacy voices table (migration 000003) is unused; code only uses user_voices.
-- Drop to clean unused database assets.
DROP TABLE IF EXISTS voices;
