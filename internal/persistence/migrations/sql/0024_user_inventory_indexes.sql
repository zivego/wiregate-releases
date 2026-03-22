CREATE INDEX IF NOT EXISTS idx_users_inventory_created
    ON users (created_at DESC, id DESC);
