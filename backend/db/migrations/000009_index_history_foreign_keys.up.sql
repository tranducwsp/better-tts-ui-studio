-- Postgres does not automatically create indexes for foreign key reference columns,
-- so history queries would otherwise perform sequential scans.
--
-- ListUserHistorySummaries joins tts_jobs with tts_chunks and performs GROUP BY: without
-- an index on tts_chunks.job_id, opening history forces a scan of the ENTIRE chunks table
-- across all users. That table grows with every chunk and never shrinks, so query cost scales
-- with global system data rather than per-user data.
--
-- The second index covers both the filter and sort columns, as queries end with
-- ORDER BY created_at DESC. ON DELETE CASCADE on both foreign keys also uses these indexes
-- when deleting users.
CREATE INDEX IF NOT EXISTS idx_tts_chunks_job_id ON tts_chunks (job_id);
CREATE INDEX IF NOT EXISTS idx_tts_jobs_user_created ON tts_jobs (user_id, created_at DESC);
