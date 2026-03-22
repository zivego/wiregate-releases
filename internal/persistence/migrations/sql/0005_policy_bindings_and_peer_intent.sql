ALTER TABLE access_policies
ADD COLUMN destinations_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE peers
ADD COLUMN allowed_ips_json TEXT NOT NULL DEFAULT '[]';

CREATE TABLE IF NOT EXISTS enrollment_token_policy_bindings (
  token_id TEXT NOT NULL,
  access_policy_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (token_id, access_policy_id),
  FOREIGN KEY(token_id) REFERENCES enrollment_tokens(id),
  FOREIGN KEY(access_policy_id) REFERENCES access_policies(id)
);
