CREATE TABLE IF NOT EXISTS analytics_rollups (
    metric TEXT NOT NULL,
    bucket TEXT NOT NULL,
    bucket_start TEXT NOT NULL,
    value INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (metric, bucket, bucket_start)
);

CREATE INDEX IF NOT EXISTS idx_analytics_rollups_metric_bucket_start
    ON analytics_rollups (metric, bucket, bucket_start DESC);
