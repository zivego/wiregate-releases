CREATE TABLE IF NOT EXISTS log_sinks (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  config_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS log_route_configs (
  id TEXT PRIMARY KEY,
  routes_json TEXT NOT NULL DEFAULT '[]',
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS log_delivery_status (
  sink_id TEXT PRIMARY KEY,
  queue_depth INTEGER NOT NULL DEFAULT 0,
  dropped_events INTEGER NOT NULL DEFAULT 0,
  total_delivered INTEGER NOT NULL DEFAULT 0,
  total_failed INTEGER NOT NULL DEFAULT 0,
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  last_attempted_at TEXT,
  last_delivered_at TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  FOREIGN KEY(sink_id) REFERENCES log_sinks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_log_sinks_type_enabled ON log_sinks(type, enabled);
