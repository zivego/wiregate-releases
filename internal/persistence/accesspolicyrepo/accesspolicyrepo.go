package accesspolicyrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Policy mirrors the access_policies table.
type Policy struct {
	ID               string
	Name             string
	Description      string
	DestinationsJSON string
	TrafficMode      string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Repo provides access policy persistence operations.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Insert(ctx context.Context, policy Policy) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO access_policies (id, name, description, destinations_json, traffic_mode, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		policy.ID,
		policy.Name,
		nullString(policy.Description),
		policy.DestinationsJSON,
		policy.TrafficMode,
		policy.CreatedAt.UTC().Format(time.RFC3339),
		policy.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("accesspolicyrepo insert: %w", err)
	}
	return nil
}

func (r *Repo) FindByID(ctx context.Context, id string) (*Policy, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, destinations_json, traffic_mode, created_at, updated_at
		   FROM access_policies
		  WHERE id = ?
		  LIMIT 1`,
		id,
	)
	policy, err := scanPolicy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("accesspolicyrepo find by id: %w", err)
	}
	return policy, nil
}

func (r *Repo) FindByIDs(ctx context.Context, ids []string) ([]Policy, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	query, args := buildInClause(
		`SELECT id, name, description, destinations_json, traffic_mode, created_at, updated_at
		   FROM access_policies
		  WHERE id IN (`,
		ids,
	)
	rows, err := r.db.QueryContext(ctx, query+")", args...)
	if err != nil {
		return nil, fmt.Errorf("accesspolicyrepo find by ids: %w", err)
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		policy, err := scanPolicy(rows)
		if err != nil {
			return nil, fmt.Errorf("accesspolicyrepo find by ids scan: %w", err)
		}
		policies = append(policies, *policy)
	}
	return policies, rows.Err()
}

func (r *Repo) List(ctx context.Context) ([]Policy, error) {
	policies, _, err := r.ListPage(ctx, ListFilter{})
	return policies, err
}

func (r *Repo) ListPage(ctx context.Context, filter ListFilter) ([]Policy, bool, error) {
	query := `SELECT id, name, description, destinations_json, traffic_mode, created_at, updated_at
		FROM access_policies`
	var clauses []string
	var args []any
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
		return nil, false, fmt.Errorf("accesspolicyrepo list: %w", err)
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		policy, err := scanPolicy(rows)
		if err != nil {
			return nil, false, fmt.Errorf("accesspolicyrepo list scan: %w", err)
		}
		policies = append(policies, *policy)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(policies) > limit
	if hasMore {
		policies = policies[:limit]
	}
	return policies, hasMore, nil
}

func (r *Repo) Update(ctx context.Context, policy Policy) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE access_policies
		    SET name = ?, description = ?, destinations_json = ?, traffic_mode = ?, updated_at = ?
		  WHERE id = ?`,
		policy.Name,
		nullString(policy.Description),
		policy.DestinationsJSON,
		policy.TrafficMode,
		policy.UpdatedAt.UTC().Format(time.RFC3339),
		policy.ID,
	)
	if err != nil {
		return fmt.Errorf("accesspolicyrepo update: %w", err)
	}
	return nil
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func buildInClause(prefix string, ids []string) (string, []any) {
	args := make([]any, 0, len(ids))
	query := prefix
	for i, id := range ids {
		if i > 0 {
			query += ", "
		}
		query += "?"
		args = append(args, id)
	}
	return query, args
}

type scanner interface {
	Scan(dest ...any) error
}

type ListFilter struct {
	Limit      int
	CursorID   string
	CursorTime time.Time
}

func scanPolicy(row scanner) (*Policy, error) {
	var policy Policy
	var description sql.NullString
	var createdAt string
	var updatedAt string
	err := row.Scan(&policy.ID, &policy.Name, &description, &policy.DestinationsJSON, &policy.TrafficMode, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	policy.Description = description.String
	policy.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	policy.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &policy, nil
}
