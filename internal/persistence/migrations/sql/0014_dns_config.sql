CREATE TABLE IF NOT EXISTS dns_configs (
  id TEXT PRIMARY KEY,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  servers_json TEXT NOT NULL DEFAULT '[]',
  search_domains_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
