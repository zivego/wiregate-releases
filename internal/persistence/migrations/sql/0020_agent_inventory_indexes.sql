CREATE INDEX IF NOT EXISTS idx_agents_inventory_created
    ON agents (status, platform, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_agents_inventory_hostname
    ON agents (hostname, created_at DESC, id DESC);
