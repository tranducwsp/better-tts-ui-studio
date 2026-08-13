-- 000010_create_auth_sessions_table.up.sql

-- Refresh token registry: each row = ONE issued refresh token belonging to a
-- login session (session_family_id). Previously refresh tokens were stateless 30-day JWTs
-- with no revocation mechanism — logout only cleared cookies while the token remained server-valid.
--
-- Rotation: each refresh creates a NEW row (new jti) in the same family, marking the old row
-- revoked_at + replaced_by. Exactly one active row exists per family at any given time.
--
-- Replay: presenting an old token (revoked + replaced) is rejected. For multi-tab browser access,
-- shared cookies ensure retries use the latest token without killing the family on race conditions.
CREATE TABLE IF NOT EXISTS auth_sessions (
    id                VARCHAR(64) PRIMARY KEY,  -- Refresh JWT jti
    session_family_id VARCHAR(64) NOT NULL,     -- 1 login = 1 family; preserved across rotation
    user_id           VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at        TIMESTAMPTZ NOT NULL,     -- ABSOLUTE expiration, not extended by rotation
    revoked_at        TIMESTAMPTZ,              -- NULL = active
    replaced_by       VARCHAR(64),              -- Replacement token jti (rotation)
    user_agent        TEXT,
    ip                VARCHAR(64)
);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_user   ON auth_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_family ON auth_sessions (session_family_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires ON auth_sessions (expires_at);
