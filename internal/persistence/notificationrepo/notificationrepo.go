// Package notificationrepo provides persistence for notification preferences.
package notificationrepo

import (
	"context"
	"database/sql"
	"time"

	"github.com/zivego/wiregate/internal/persistence/db"
)

// Repo manages notification preference records.
type Repo struct {
	db *db.Handle
}

// New creates a new notification preference repository.
func New(handle *db.Handle) *Repo {
	return &Repo{db: handle}
}

// IsEnabled returns whether the user has enabled the given channel.
// Defaults to true if no preference record exists.
func (r *Repo) IsEnabled(ctx context.Context, userID, channel string) (bool, error) {
	var enabled bool
	err := r.db.QueryRowContext(ctx,
		`SELECT enabled FROM notification_preferences WHERE user_id = ? AND channel = ?`,
		userID, channel).Scan(&enabled)
	if err == sql.ErrNoRows {
		return true, nil // default enabled
	}
	return enabled, err
}

// SetEnabled upserts the notification preference for a user/channel.
func (r *Repo) SetEnabled(ctx context.Context, userID, channel string, enabled bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notification_preferences (user_id, channel, enabled, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (user_id, channel) DO UPDATE SET
		   enabled = excluded.enabled,
		   updated_at = excluded.updated_at`,
		userID, channel, enabled, now)
	return err
}

// ListEnabledUsers returns user IDs that have the given channel enabled.
// An empty result means no preferences stored (caller should treat as "all enabled").
func (r *Repo) ListEnabledUsers(ctx context.Context, channel string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id FROM notification_preferences WHERE channel = ? AND enabled = TRUE`,
		channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
