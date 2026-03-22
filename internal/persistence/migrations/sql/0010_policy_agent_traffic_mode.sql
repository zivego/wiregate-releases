ALTER TABLE access_policies
ADD COLUMN traffic_mode TEXT NOT NULL DEFAULT 'standard';

ALTER TABLE agents
ADD COLUMN traffic_mode_override TEXT NOT NULL DEFAULT '';
