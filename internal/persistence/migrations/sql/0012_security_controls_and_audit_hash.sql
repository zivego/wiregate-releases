CREATE TABLE IF NOT EXISTS security_policies (
  id TEXT PRIMARY KEY,
  required_admin_amr TEXT NOT NULL DEFAULT '',
  required_admin_acr TEXT NOT NULL DEFAULT '',
  dual_approval_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS security_approvals (
  id TEXT PRIMARY KEY,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  request_payload_json TEXT NOT NULL DEFAULT '',
  requested_by_user_id TEXT NOT NULL,
  approved_by_user_id TEXT,
  rejected_by_user_id TEXT,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  decided_at TEXT,
  FOREIGN KEY(requested_by_user_id) REFERENCES users(id),
  FOREIGN KEY(approved_by_user_id) REFERENCES users(id),
  FOREIGN KEY(rejected_by_user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_security_approvals_status_created_at ON security_approvals(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_security_approvals_action_resource ON security_approvals(action, resource_type, resource_id, status);

ALTER TABLE audit_events ADD COLUMN prev_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN event_hash TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_events_event_hash ON audit_events(event_hash);
