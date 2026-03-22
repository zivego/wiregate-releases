ALTER TABLE agents ADD COLUMN reported_version TEXT;
ALTER TABLE agents ADD COLUMN reported_config_fingerprint TEXT;
ALTER TABLE agents ADD COLUMN last_apply_status TEXT;
ALTER TABLE agents ADD COLUMN last_apply_error TEXT;
ALTER TABLE agents ADD COLUMN last_applied_at TEXT;
