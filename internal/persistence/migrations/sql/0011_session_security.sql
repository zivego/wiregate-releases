ALTER TABLE sessions ADD COLUMN public_id TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN auth_provider TEXT NOT NULL DEFAULT 'password';
ALTER TABLE sessions ADD COLUMN last_seen_at TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN source_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN revoked_reason TEXT NOT NULL DEFAULT '';

UPDATE sessions
SET public_id = id
WHERE public_id = '';

UPDATE sessions
SET last_seen_at = issued_at
WHERE last_seen_at = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_public_id ON sessions(public_id);
CREATE INDEX IF NOT EXISTS idx_sessions_last_seen_at ON sessions(last_seen_at);
