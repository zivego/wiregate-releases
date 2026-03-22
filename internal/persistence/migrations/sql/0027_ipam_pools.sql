CREATE TABLE IF NOT EXISTS ipam_pools (
    id          TEXT PRIMARY KEY,
    cidr        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_ipv6     BOOLEAN NOT NULL DEFAULT FALSE,
    gateway     TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ipam_reservations (
    id         TEXT PRIMARY KEY,
    pool_id    TEXT NOT NULL REFERENCES ipam_pools(id) ON DELETE CASCADE,
    address    TEXT NOT NULL,
    peer_id    TEXT DEFAULT NULL REFERENCES peers(id) ON DELETE SET NULL,
    label      TEXT NOT NULL DEFAULT '',
    reserved_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ipam_reservations_address ON ipam_reservations(pool_id, address);
CREATE INDEX IF NOT EXISTS idx_ipam_reservations_peer ON ipam_reservations(peer_id);
