// Package ratelimitrepo provides DB-backed sliding window rate limiting.
package ratelimitrepo

import (
	"context"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Repo persists rate limit attempts in the database so that limits survive
// restarts and are shared across multiple server instances.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

// Count returns the number of attempts for key within the sliding window.
func (r *Repo) Count(ctx context.Context, key string, windowStart time.Time) (int, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM rate_limit_entries WHERE key = ? AND attempted_at > ?`,
		key, windowStart.UTC().Format(time.RFC3339Nano))
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("ratelimitrepo count: %w", err)
	}
	return n, nil
}

// Record inserts a new attempt for key.
func (r *Repo) Record(ctx context.Context, key string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO rate_limit_entries (key, attempted_at) VALUES (?, ?)`,
		key, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("ratelimitrepo record: %w", err)
	}
	return nil
}

// Cleanup deletes entries older than cutoff. Should be called periodically.
func (r *Repo) Cleanup(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM rate_limit_entries WHERE attempted_at < ?`,
		cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("ratelimitrepo cleanup: %w", err)
	}
	return result.RowsAffected()
}
