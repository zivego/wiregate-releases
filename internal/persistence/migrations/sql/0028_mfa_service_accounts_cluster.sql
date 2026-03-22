ALTER TABLE users ADD COLUMN mfa_totp_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN mfa_totp_enabled BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS service_accounts (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    role         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TEXT
);

CREATE TABLE IF NOT EXISTS service_account_keys (
    id                 TEXT PRIMARY KEY,
    service_account_id TEXT NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    name               TEXT NOT NULL DEFAULT '',
    key_prefix         TEXT NOT NULL,
    token_hash         TEXT NOT NULL UNIQUE,
    status             TEXT NOT NULL DEFAULT 'active',
    expires_at         TEXT,
    created_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at         TEXT,
    last_used_at       TEXT
);

CREATE INDEX IF NOT EXISTS idx_service_account_keys_account ON service_account_keys(service_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_service_account_keys_status ON service_account_keys(status, created_at DESC);

CREATE TABLE IF NOT EXISTS cluster_leases (
    lease_name       TEXT PRIMARY KEY,
    leader_id        TEXT NOT NULL,
    lease_expires_at TEXT NOT NULL,
    updated_at       TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cluster_nodes (
    instance_id       TEXT PRIMARY KEY,
    is_leader         BOOLEAN NOT NULL DEFAULT FALSE,
    leader_id         TEXT NOT NULL DEFAULT '',
    last_heartbeat_at TEXT NOT NULL,
    lease_expires_at  TEXT NOT NULL DEFAULT '',
    updated_at        TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
