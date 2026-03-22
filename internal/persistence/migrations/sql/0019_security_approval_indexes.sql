CREATE INDEX IF NOT EXISTS idx_security_approvals_created
    ON security_approvals (status, created_at DESC, id DESC);
