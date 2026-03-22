// Package metricsrepo provides aggregate count queries for Prometheus metrics.
package metricsrepo

import (
	"context"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Repo runs lightweight aggregate queries used by the metrics collector.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) AgentCountsByStatus(ctx context.Context) (map[string]int, error) {
	return r.countByColumn(ctx, "agents", "status")
}

func (r *Repo) PeerCountsByStatus(ctx context.Context) (map[string]int, error) {
	return r.countByColumn(ctx, "peers", "status")
}

func (r *Repo) ActiveSessionCount(ctx context.Context) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	row := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE revoked_at IS NULL AND expires_at > ?`, now)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("metricsrepo active session count: %w", err)
	}
	return n, nil
}

func (r *Repo) EnrollmentTokenCountsByStatus(ctx context.Context) (map[string]int, error) {
	return r.countByColumn(ctx, "enrollment_tokens", "status")
}

func (r *Repo) countByColumn(ctx context.Context, table, column string) (map[string]int, error) {
	query := fmt.Sprintf("SELECT %s, COUNT(*) FROM %s GROUP BY %s", column, table, column)
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("metricsrepo count %s.%s: %w", table, column, err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var label string
		var n int
		if err := rows.Scan(&label, &n); err != nil {
			return nil, fmt.Errorf("metricsrepo count %s.%s scan: %w", table, column, err)
		}
		counts[label] = n
	}
	return counts, rows.Err()
}
