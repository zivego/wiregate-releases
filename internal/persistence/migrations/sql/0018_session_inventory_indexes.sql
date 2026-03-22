CREATE INDEX IF NOT EXISTS idx_sessions_inventory_global
    ON sessions (revoked_at, last_seen_at DESC, issued_at DESC, public_id DESC);

CREATE INDEX IF NOT EXISTS idx_sessions_inventory_by_user
    ON sessions (user_id, revoked_at, last_seen_at DESC, issued_at DESC, public_id DESC);
