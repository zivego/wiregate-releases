CREATE INDEX IF NOT EXISTS idx_peers_inventory_created
    ON peers (status, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_peers_inventory_agent
    ON peers (agent_id, created_at DESC, id DESC);
