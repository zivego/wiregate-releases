CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_inventory_created
    ON enrollment_tokens (created_at DESC, id DESC);
