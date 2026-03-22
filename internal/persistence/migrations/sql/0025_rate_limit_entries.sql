CREATE TABLE IF NOT EXISTS rate_limit_entries (
    key          TEXT NOT NULL,
    attempted_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_key_time
    ON rate_limit_entries (key, attempted_at);
