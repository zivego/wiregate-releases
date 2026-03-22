package capacityrepo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

type Snapshot struct {
	TotalUsers               int
	TotalAgents              int
	TotalPeers               int
	TotalAccessPolicies      int
	TotalPolicyAssignments   int
	TotalEnrollmentTokens    int
	TotalAuditEvents         int
	PendingSecurityApprovals int
	RecentlySeenAgents       int
	FailedAgents             int
	DriftedPeers             int
	ActiveSessions           int
	OldestAuditAt            *time.Time
	NewestAuditAt            *time.Time
	TotalAnalyticsRollups    int
	TotalLogDeadLetters      int
}

type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Engine() string {
	if r == nil || r.db == nil {
		return string(persistdb.EngineSQLite)
	}
	return string(r.db.Engine())
}

func (r *Repo) LoadSnapshot(ctx context.Context, now, idleCutoff, recentAgentCutoff time.Time) (Snapshot, error) {
	if r == nil || r.db == nil {
		return Snapshot{}, fmt.Errorf("capacityrepo is not configured")
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM agents),
			(SELECT COUNT(*) FROM peers),
			(SELECT COUNT(*) FROM access_policies),
			(SELECT COUNT(*) FROM policy_assignments),
			(SELECT COUNT(*) FROM enrollment_tokens),
			(SELECT COUNT(*) FROM audit_events),
			(SELECT COUNT(*) FROM security_approvals WHERE status = 'pending'),
			(SELECT COUNT(*) FROM agents WHERE last_seen_at IS NOT NULL AND last_seen_at >= ?),
			(SELECT COUNT(*) FROM agents WHERE last_apply_status = 'apply_failed'),
			(SELECT COUNT(*) FROM runtime_sync_state WHERE drift_state <> 'in_sync'),
			(SELECT COUNT(*) FROM sessions WHERE revoked_at IS NULL AND expires_at > ? AND last_seen_at > ?),
			(SELECT MIN(created_at) FROM audit_events),
			(SELECT MAX(created_at) FROM audit_events),
			(SELECT COUNT(*) FROM analytics_rollups),
			(SELECT COUNT(*) FROM log_delivery_dead_letters)
	`,
		recentAgentCutoff.UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339),
		idleCutoff.UTC().Format(time.RFC3339),
	)

	var snapshot Snapshot
	var oldestAudit sql.NullString
	var newestAudit sql.NullString
	if err := row.Scan(
		&snapshot.TotalUsers,
		&snapshot.TotalAgents,
		&snapshot.TotalPeers,
		&snapshot.TotalAccessPolicies,
		&snapshot.TotalPolicyAssignments,
		&snapshot.TotalEnrollmentTokens,
		&snapshot.TotalAuditEvents,
		&snapshot.PendingSecurityApprovals,
		&snapshot.RecentlySeenAgents,
		&snapshot.FailedAgents,
		&snapshot.DriftedPeers,
		&snapshot.ActiveSessions,
		&oldestAudit,
		&newestAudit,
		&snapshot.TotalAnalyticsRollups,
		&snapshot.TotalLogDeadLetters,
	); err != nil {
		return Snapshot{}, fmt.Errorf("capacityrepo load snapshot: %w", err)
	}

	if oldestAudit.Valid {
		parsed := parseTime(oldestAudit.String)
		snapshot.OldestAuditAt = &parsed
	}
	if newestAudit.Valid {
		parsed := parseTime(newestAudit.String)
		snapshot.NewestAuditAt = &parsed
	}

	return snapshot, nil
}

func parseTime(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
