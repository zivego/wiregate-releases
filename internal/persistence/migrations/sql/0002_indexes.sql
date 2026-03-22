CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_status ON enrollment_tokens(status);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_peers_agent_id ON peers(agent_id);
CREATE INDEX IF NOT EXISTS idx_policy_assignments_agent_id ON policy_assignments(agent_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at);
