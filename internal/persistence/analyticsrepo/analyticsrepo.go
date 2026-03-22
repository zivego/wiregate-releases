package analyticsrepo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

type AuditEvent struct {
	ID           string
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	CreatedAt    time.Time
}

type EnrollmentToken struct {
	ID        string
	CreatedAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
	ExpiresAt time.Time
}

type Agent struct {
	ID              string
	Hostname        string
	Platform        string
	Status          string
	LastSeenAt      *time.Time
	LastApplyStatus string
	LastApplyError  string
	PeerID          string
}

type RuntimeSyncState struct {
	ID               string
	PeerID           string
	DriftState       string
	LastObservedAt   time.Time
	LastReconciledAt *time.Time
	DetailsJSON      string
}

type PolicyAssignment struct {
	ID             string
	AgentID        string
	AccessPolicyID string
	Status         string
	CreatedAt      time.Time
}

type RollupPoint struct {
	Metric      string
	Bucket      string
	BucketStart time.Time
	Value       int
	UpdatedAt   time.Time
}

// Repo provides read access for analytics aggregation.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) LoadAuditEventsSince(ctx context.Context, start time.Time) ([]AuditEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, actor_user_id, action, resource_type, resource_id, result, created_at
		   FROM audit_events
		  ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("analyticsrepo load audit events: %w", err)
	}
	defer rows.Close()

	var events []AuditEvent
	for rows.Next() {
		var event AuditEvent
		var actor sql.NullString
		var resourceID sql.NullString
		var createdAt string
		if err := rows.Scan(&event.ID, &actor, &event.Action, &event.ResourceType, &resourceID, &event.Result, &createdAt); err != nil {
			return nil, fmt.Errorf("analyticsrepo scan audit event: %w", err)
		}
		event.ActorUserID = actor.String
		event.ResourceID = resourceID.String
		event.CreatedAt = parseTimestamp(createdAt)
		if event.CreatedAt.Before(start) {
			continue
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *Repo) LoadEnrollmentTokens(ctx context.Context) ([]EnrollmentToken, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, created_at, used_at, revoked_at, expires_at
		   FROM enrollment_tokens
		  ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("analyticsrepo load enrollment tokens: %w", err)
	}
	defer rows.Close()

	var tokens []EnrollmentToken
	for rows.Next() {
		var token EnrollmentToken
		var createdAt string
		var usedAt sql.NullString
		var revokedAt sql.NullString
		var expiresAt string
		if err := rows.Scan(&token.ID, &createdAt, &usedAt, &revokedAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("analyticsrepo scan enrollment token: %w", err)
		}
		token.CreatedAt = parseTimestamp(createdAt)
		token.ExpiresAt = parseTimestamp(expiresAt)
		if usedAt.Valid {
			parsed := parseTimestamp(usedAt.String)
			token.UsedAt = &parsed
		}
		if revokedAt.Valid {
			parsed := parseTimestamp(revokedAt.String)
			token.RevokedAt = &parsed
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (r *Repo) LoadAgents(ctx context.Context) ([]Agent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT a.id, a.hostname, a.platform, a.status, a.last_seen_at, a.last_apply_status, a.last_apply_error, p.id
		   FROM agents a
		   LEFT JOIN peers p ON p.agent_id = a.id
		  ORDER BY a.created_at DESC, a.id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("analyticsrepo load agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var agent Agent
		var lastSeenAt sql.NullString
		var lastApplyStatus sql.NullString
		var lastApplyError sql.NullString
		var peerID sql.NullString
		if err := rows.Scan(&agent.ID, &agent.Hostname, &agent.Platform, &agent.Status, &lastSeenAt, &lastApplyStatus, &lastApplyError, &peerID); err != nil {
			return nil, fmt.Errorf("analyticsrepo scan agent: %w", err)
		}
		if lastSeenAt.Valid {
			parsed := parseTimestamp(lastSeenAt.String)
			agent.LastSeenAt = &parsed
		}
		agent.LastApplyStatus = lastApplyStatus.String
		agent.LastApplyError = lastApplyError.String
		agent.PeerID = peerID.String
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

func (r *Repo) LoadRuntimeSyncStates(ctx context.Context) ([]RuntimeSyncState, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, peer_id, drift_state, last_observed_at, last_reconciled_at, details
		   FROM runtime_sync_state
		  ORDER BY last_observed_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("analyticsrepo load runtime sync state: %w", err)
	}
	defer rows.Close()

	var states []RuntimeSyncState
	for rows.Next() {
		var state RuntimeSyncState
		var peerID sql.NullString
		var lastObservedAt string
		var lastReconciledAt sql.NullString
		var details sql.NullString
		if err := rows.Scan(&state.ID, &peerID, &state.DriftState, &lastObservedAt, &lastReconciledAt, &details); err != nil {
			return nil, fmt.Errorf("analyticsrepo scan runtime sync state: %w", err)
		}
		state.PeerID = peerID.String
		state.LastObservedAt = parseTimestamp(lastObservedAt)
		if lastReconciledAt.Valid {
			parsed := parseTimestamp(lastReconciledAt.String)
			state.LastReconciledAt = &parsed
		}
		state.DetailsJSON = details.String
		states = append(states, state)
	}
	return states, rows.Err()
}

func (r *Repo) LoadPolicyAssignments(ctx context.Context) ([]PolicyAssignment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, agent_id, access_policy_id, status, created_at
		   FROM policy_assignments
		  ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("analyticsrepo load policy assignments: %w", err)
	}
	defer rows.Close()

	var assignments []PolicyAssignment
	for rows.Next() {
		var assignment PolicyAssignment
		var createdAt string
		if err := rows.Scan(&assignment.ID, &assignment.AgentID, &assignment.AccessPolicyID, &assignment.Status, &createdAt); err != nil {
			return nil, fmt.Errorf("analyticsrepo scan policy assignment: %w", err)
		}
		assignment.CreatedAt = parseTimestamp(createdAt)
		assignments = append(assignments, assignment)
	}
	return assignments, rows.Err()
}

func (r *Repo) CountPolicies(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM access_policies`).Scan(&count); err != nil {
		return 0, fmt.Errorf("analyticsrepo count policies: %w", err)
	}
	return count, nil
}

func (r *Repo) CountAuditEvents(ctx context.Context, start, end time.Time, authSecurityOnly bool) (int, error) {
	query := `SELECT COUNT(*) FROM audit_events WHERE created_at >= ? AND created_at < ?`
	args := []any{
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
	}
	if authSecurityOnly {
		query += ` AND (action LIKE ? OR action LIKE ? OR action LIKE ?)`
		args = append(args, "auth.%", "security.%", "session.%")
	}
	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("analyticsrepo count audit events: %w", err)
	}
	return count, nil
}

func (r *Repo) LoadRollupSeries(ctx context.Context, metric, bucket string, start, end time.Time) ([]RollupPoint, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT metric, bucket, bucket_start, value, updated_at
		   FROM analytics_rollups
		  WHERE metric = ?
		    AND bucket = ?
		    AND bucket_start >= ?
		    AND bucket_start < ?
		  ORDER BY bucket_start ASC`,
		metric,
		bucket,
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("analyticsrepo load rollup series: %w", err)
	}
	defer rows.Close()

	var points []RollupPoint
	for rows.Next() {
		var point RollupPoint
		var bucketStart string
		var updatedAt string
		if err := rows.Scan(&point.Metric, &point.Bucket, &bucketStart, &point.Value, &updatedAt); err != nil {
			return nil, fmt.Errorf("analyticsrepo scan rollup point: %w", err)
		}
		point.BucketStart = parseTimestamp(bucketStart)
		point.UpdatedAt = parseTimestamp(updatedAt)
		points = append(points, point)
	}
	return points, rows.Err()
}

func (r *Repo) ReplaceRollupSeries(ctx context.Context, metric, bucket string, start, end, updatedAt time.Time, points []RollupPoint) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("analyticsrepo begin rollup tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM analytics_rollups
		  WHERE metric = ?
		    AND bucket = ?
		    AND bucket_start >= ?
		    AND bucket_start < ?`,
	), metric, bucket, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("analyticsrepo delete rollup series: %w", err)
	}

	for _, point := range points {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(
			`INSERT INTO analytics_rollups (metric, bucket, bucket_start, value, updated_at)
			 VALUES (?, ?, ?, ?, ?)`,
		), metric, bucket, point.BucketStart.UTC().Format(time.RFC3339Nano), point.Value, updatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("analyticsrepo insert rollup point: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("analyticsrepo commit rollup tx: %w", err)
	}
	return nil
}

func parseTimestamp(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
