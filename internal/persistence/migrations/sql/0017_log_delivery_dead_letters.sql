CREATE TABLE IF NOT EXISTS log_delivery_dead_letters (
    id TEXT PRIMARY KEY,
    sink_id TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    event_json TEXT NOT NULL,
    error_message TEXT NOT NULL,
    test_delivery BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (sink_id) REFERENCES log_sinks (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_log_delivery_dead_letters_sink_created
    ON log_delivery_dead_letters (sink_id, created_at DESC);
