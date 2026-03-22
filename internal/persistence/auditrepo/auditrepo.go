package auditrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Event mirrors the audit_events table.
type Event struct {
	ID           string
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	CreatedAt    time.Time
	MetadataJSON string
	PrevHash     string
	EventHash    string
}

// ListFilter limits query results.
type ListFilter struct {
	Action       string
	ResourceType string
	Result       string
	ActorUserID  string
	Limit        int
	CursorID     string
	CursorTime   time.Time
}

// Repo provides audit event persistence.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

// Insert appends a new immutable event row.
func (r *Repo) Insert(ctx context.Context, event Event) error {
	now := event.CreatedAt.UTC().Format(time.RFC3339Nano)
	var actor any
	if event.ActorUserID != "" {
		actor = event.ActorUserID
	}
	var resourceID any
	if event.ResourceID != "" {
		resourceID = event.ResourceID
	}
	var metadata any
	if event.MetadataJSON != "" {
		metadata = event.MetadataJSON
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_events (
			id, actor_user_id, action, resource_type, resource_id, result, created_at, metadata_json, prev_hash, event_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, actor, event.Action, event.ResourceType, resourceID, event.Result, now, metadata, event.PrevHash, event.EventHash,
	)
	if err != nil {
		return fmt.Errorf("auditrepo insert: %w", err)
	}
	return nil
}

func (r *Repo) LatestHash(ctx context.Context) (string, error) {
	var value sql.NullString
	if err := r.db.QueryRowContext(ctx,
		`SELECT event_hash
		   FROM audit_events
		  WHERE event_hash <> ''
		  ORDER BY created_at DESC, id DESC
		  LIMIT 1`,
	).Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("auditrepo latest hash: %w", err)
	}
	return value.String, nil
}

// List returns audit events ordered from newest to oldest.
func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Event, error) {
	events, _, err := r.ListPage(ctx, filter)
	return events, err
}

func (r *Repo) ListPage(ctx context.Context, filter ListFilter) ([]Event, bool, error) {
	query := `SELECT id, actor_user_id, action, resource_type, resource_id, result, created_at, metadata_json, prev_hash, event_hash
		FROM audit_events`
	var clauses []string
	var args []any

	if filter.Action != "" {
		if strings.HasSuffix(filter.Action, "*") || strings.HasSuffix(filter.Action, ".") {
			prefix := strings.TrimSuffix(strings.TrimSuffix(filter.Action, "*"), ".")
			clauses = append(clauses, "action LIKE ?")
			args = append(args, prefix+".%")
		} else {
			clauses = append(clauses, "action = ?")
			args = append(args, filter.Action)
		}
	}
	if filter.ResourceType != "" {
		clauses = append(clauses, "resource_type = ?")
		args = append(args, filter.ResourceType)
	}
	if filter.Result != "" {
		clauses = append(clauses, "result = ?")
		args = append(args, filter.Result)
	}
	if filter.ActorUserID != "" {
		clauses = append(clauses, "actor_user_id = ?")
		args = append(args, filter.ActorUserID)
	}
	if !filter.CursorTime.IsZero() && strings.TrimSpace(filter.CursorID) != "" {
		clauses = append(clauses, "(created_at < ? OR (created_at = ? AND id < ?))")
		cursorTime := filter.CursorTime.UTC().Format(time.RFC3339Nano)
		args = append(args, cursorTime, cursorTime, strings.TrimSpace(filter.CursorID))
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query += " LIMIT ?"
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("auditrepo list: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		var actorUserID sql.NullString
		var resourceID sql.NullString
		var createdAt string
		var metadataJSON sql.NullString
		var prevHash string
		var eventHash string
		if err := rows.Scan(
			&event.ID,
			&actorUserID,
			&event.Action,
			&event.ResourceType,
			&resourceID,
			&event.Result,
			&createdAt,
			&metadataJSON,
			&prevHash,
			&eventHash,
		); err != nil {
			return nil, false, fmt.Errorf("auditrepo list scan: %w", err)
		}
		event.ActorUserID = actorUserID.String
		event.ResourceID = resourceID.String
		event.MetadataJSON = metadataJSON.String
		event.PrevHash = prevHash
		event.EventHash = eventHash
		event.CreatedAt = parseAuditTimestamp(createdAt)
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	return events, hasMore, nil
}

func parseAuditTimestamp(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
