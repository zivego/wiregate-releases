-- MVP base schema for SQLite.
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  role TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  issued_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS enrollment_tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  model TEXT NOT NULL,
  scope TEXT NOT NULL,
  status TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at TEXT,
  revoked_at TEXT,
  created_by_user_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(created_by_user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS agents (
  id TEXT PRIMARY KEY,
  hostname TEXT NOT NULL,
  platform TEXT NOT NULL,
  status TEXT NOT NULL,
  last_seen_at TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS peers (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  public_key TEXT NOT NULL,
  assigned_address TEXT,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(agent_id) REFERENCES agents(id)
);

CREATE TABLE IF NOT EXISTS access_policies (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS policy_assignments (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  access_policy_id TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(agent_id) REFERENCES agents(id),
  FOREIGN KEY(access_policy_id) REFERENCES access_policies(id)
);

CREATE TABLE IF NOT EXISTS key_metadata (
  id TEXT PRIMARY KEY,
  peer_id TEXT NOT NULL,
  storage_location TEXT NOT NULL,
  encryption_state TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(peer_id) REFERENCES peers(id)
);

CREATE TABLE IF NOT EXISTS runtime_sync_state (
  id TEXT PRIMARY KEY,
  peer_id TEXT,
  drift_state TEXT NOT NULL,
  last_observed_at TEXT NOT NULL,
  last_reconciled_at TEXT,
  details TEXT
);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  actor_user_id TEXT,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  result TEXT NOT NULL,
  created_at TEXT NOT NULL,
  metadata_json TEXT
);
