package agentrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Agent mirrors the agents table.
type Agent struct {
	ID                        string
	Hostname                  string
	Platform                  string
	Status                    string
	GatewayMode               string
	TrafficModeOverride       string
	AuthTokenHash             string
	LastSeenAt                *time.Time
	ReportedVersion           string
	ReportedConfigFingerprint string
	LastApplyStatus           string
	LastApplyError            string
	LastAppliedAt             *time.Time
	CreatedAt                 time.Time
}

// ListFilter limits inventory results.
type ListFilter struct {
	Status     string
	Platform   string
	Query      string
	Limit      int
	CursorID   string
	CursorTime time.Time
}

// Repo provides agent persistence operations.
type Repo struct {
	db *persistdb.Handle
}

type CheckInStatus struct {
	LastSeenAt                time.Time
	ReportedVersion           string
	ReportedConfigFingerprint string
	LastApplyStatus           string
	LastApplyError            string
	LastAppliedAt             *time.Time
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Insert(ctx context.Context, agent Agent) error {
	if strings.TrimSpace(agent.GatewayMode) == "" {
		agent.GatewayMode = "disabled"
	}
	var lastSeen any
	if agent.LastSeenAt != nil {
		lastSeen = agent.LastSeenAt.UTC().Format(time.RFC3339)
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO agents (id, hostname, platform, status, gateway_mode, traffic_mode_override, auth_token_hash, last_seen_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.ID,
		agent.Hostname,
		agent.Platform,
		agent.Status,
		agent.GatewayMode,
		agent.TrafficModeOverride,
		agent.AuthTokenHash,
		lastSeen,
		agent.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("agentrepo insert: %w", err)
	}
	return nil
}

func (r *Repo) FindByID(ctx context.Context, id string) (*Agent, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, hostname, platform, status, gateway_mode, traffic_mode_override, auth_token_hash, last_seen_at, reported_version, reported_config_fingerprint, last_apply_status, last_apply_error, last_applied_at, created_at
		   FROM agents
		  WHERE id = ?
		  LIMIT 1`,
		id,
	)
	agent, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("agentrepo find by id: %w", err)
	}
	return agent, nil
}

func (r *Repo) FindByHostname(ctx context.Context, hostname string) (*Agent, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, hostname, platform, status, gateway_mode, traffic_mode_override, auth_token_hash, last_seen_at, reported_version, reported_config_fingerprint, last_apply_status, last_apply_error, last_applied_at, created_at
		   FROM agents
		  WHERE hostname = ?
		  LIMIT 1`,
		hostname,
	)
	agent, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("agentrepo find by hostname: %w", err)
	}
	return agent, nil
}

func (r *Repo) FindByIDs(ctx context.Context, ids []string) (map[string]Agent, error) {
	if len(ids) == 0 {
		return map[string]Agent{}, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	if len(placeholders) == 0 {
		return map[string]Agent{}, nil
	}

	query := `SELECT id, hostname, platform, status, gateway_mode, traffic_mode_override, auth_token_hash, last_seen_at, reported_version, reported_config_fingerprint, last_apply_status, last_apply_error, last_applied_at, created_at
		FROM agents
		WHERE id IN (` + strings.Join(placeholders, ", ") + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("agentrepo find by ids: %w", err)
	}
	defer rows.Close()

	agents := make(map[string]Agent, len(ids))
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, fmt.Errorf("agentrepo find by ids scan: %w", err)
		}
		agents[agent.ID] = *agent
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agentrepo find by ids rows: %w", err)
	}
	return agents, nil
}

func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Agent, error) {
	agents, _, err := r.ListPage(ctx, filter)
	return agents, err
}

func (r *Repo) ListPage(ctx context.Context, filter ListFilter) ([]Agent, bool, error) {
	query := `SELECT id, hostname, platform, status, gateway_mode, traffic_mode_override, auth_token_hash, last_seen_at, reported_version, reported_config_fingerprint, last_apply_status, last_apply_error, last_applied_at, created_at FROM agents`
	var clauses []string
	var args []any

	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Platform != "" {
		clauses = append(clauses, "platform = ?")
		args = append(args, filter.Platform)
	}
	if filter.Query != "" {
		clauses = append(clauses, "hostname LIKE ?")
		args = append(args, "%"+filter.Query+"%")
	}
	if strings.TrimSpace(filter.CursorID) != "" && !filter.CursorTime.IsZero() {
		cursorTime := filter.CursorTime.UTC().Format(time.RFC3339)
		clauses = append(clauses, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, cursorTime, cursorTime, strings.TrimSpace(filter.CursorID))
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC"
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query += " LIMIT ?"
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("agentrepo list: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, false, fmt.Errorf("agentrepo list scan: %w", err)
		}
		agents = append(agents, *agent)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}
	return agents, hasMore, nil
}

func (r *Repo) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents
		    SET status = ?
		  WHERE id = ?`,
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf("agentrepo update status: %w", err)
	}
	return nil
}

func (r *Repo) UpdateTrafficModeOverride(ctx context.Context, id, mode string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents
		    SET traffic_mode_override = ?
		  WHERE id = ?`,
		mode,
		id,
	)
	if err != nil {
		return fmt.Errorf("agentrepo update traffic mode override: %w", err)
	}
	return nil
}

func (r *Repo) UpdateGatewayMode(ctx context.Context, id, mode string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents
		    SET gateway_mode = ?
		  WHERE id = ?`,
		mode,
		id,
	)
	if err != nil {
		return fmt.Errorf("agentrepo update gateway mode: %w", err)
	}
	return nil
}

func (r *Repo) ClearAuthToken(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents
		    SET auth_token_hash = ''
		  WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("agentrepo clear auth token: %w", err)
	}
	return nil
}

func (r *Repo) UpdateCheckIn(ctx context.Context, id string, status CheckInStatus) error {
	var lastAppliedAt any
	if status.LastAppliedAt != nil {
		lastAppliedAt = status.LastAppliedAt.UTC().Format(time.RFC3339)
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents
		    SET last_seen_at = ?,
		        reported_version = ?,
		        reported_config_fingerprint = ?,
		        last_apply_status = ?,
		        last_apply_error = ?,
		        last_applied_at = ?
		  WHERE id = ?`,
		status.LastSeenAt.UTC().Format(time.RFC3339),
		status.ReportedVersion,
		status.ReportedConfigFingerprint,
		status.LastApplyStatus,
		status.LastApplyError,
		lastAppliedAt,
		id,
	)
	if err != nil {
		return fmt.Errorf("agentrepo update check in: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAgent(row scanner) (*Agent, error) {
	var agent Agent
	var authTokenHash string
	var gatewayMode string
	var trafficModeOverride string
	var lastSeen sql.NullString
	var reportedVersion sql.NullString
	var reportedConfigFingerprint sql.NullString
	var lastApplyStatus sql.NullString
	var lastApplyError sql.NullString
	var lastAppliedAt sql.NullString
	var createdAt string
	err := row.Scan(&agent.ID, &agent.Hostname, &agent.Platform, &agent.Status, &gatewayMode, &trafficModeOverride, &authTokenHash, &lastSeen, &reportedVersion, &reportedConfigFingerprint, &lastApplyStatus, &lastApplyError, &lastAppliedAt, &createdAt)
	if err != nil {
		return nil, err
	}
	agent.GatewayMode = gatewayMode
	agent.TrafficModeOverride = trafficModeOverride
	agent.AuthTokenHash = authTokenHash
	if lastSeen.Valid {
		parsed, _ := time.Parse(time.RFC3339, lastSeen.String)
		agent.LastSeenAt = &parsed
	}
	agent.ReportedVersion = reportedVersion.String
	agent.ReportedConfigFingerprint = reportedConfigFingerprint.String
	agent.LastApplyStatus = lastApplyStatus.String
	agent.LastApplyError = lastApplyError.String
	if lastAppliedAt.Valid {
		parsed, _ := time.Parse(time.RFC3339, lastAppliedAt.String)
		agent.LastAppliedAt = &parsed
	}
	agent.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &agent, nil
}
