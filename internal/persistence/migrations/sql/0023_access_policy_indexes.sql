CREATE INDEX IF NOT EXISTS idx_access_policies_inventory_created
    ON access_policies (created_at DESC, id DESC);
