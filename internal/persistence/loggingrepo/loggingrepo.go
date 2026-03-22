package loggingrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

const routeConfigID = "default"

type Sink struct {
	ID         string
	Name       string
	Type       string
	Enabled    bool
	ConfigJSON string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type RouteConfig struct {
	ID         string
	RoutesJSON string
	UpdatedAt  time.Time
}

type DeliveryStatus struct {
	SinkID              string
	QueueDepth          int
	DroppedEvents       int
	TotalDelivered      int
	TotalFailed         int
	ConsecutiveFailures int
	LastAttemptedAt     *time.Time
	LastDeliveredAt     *time.Time
	LastError           string
	UpdatedAt           time.Time
}

type DeadLetter struct {
	ID           string
	SinkID       string
	OccurredAt   time.Time
	EventJSON    string
	ErrorMessage string
	TestDelivery bool
	CreatedAt    time.Time
}

type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) ListSinks(ctx context.Context) ([]Sink, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, type, enabled, config_json, created_at, updated_at
		   FROM log_sinks
		  ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("loggingrepo list sinks: %w", err)
	}
	defer rows.Close()

	var sinks []Sink
	for rows.Next() {
		sink, err := scanSink(rows)
		if err != nil {
			return nil, fmt.Errorf("loggingrepo list sinks scan: %w", err)
		}
		sinks = append(sinks, *sink)
	}
	return sinks, rows.Err()
}

func (r *Repo) FindSinkByID(ctx context.Context, id string) (*Sink, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, type, enabled, config_json, created_at, updated_at
		   FROM log_sinks
		  WHERE id = ?
		  LIMIT 1`,
		id,
	)
	sink, err := scanSink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loggingrepo find sink: %w", err)
	}
	return sink, nil
}

func (r *Repo) UpsertSink(ctx context.Context, sink Sink) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO log_sinks (id, name, type, enabled, config_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name,
		   type = excluded.type,
		   enabled = excluded.enabled,
		   config_json = excluded.config_json,
		   updated_at = excluded.updated_at`,
		sink.ID,
		sink.Name,
		sink.Type,
		sink.Enabled,
		sink.ConfigJSON,
		sink.CreatedAt.UTC().Format(time.RFC3339Nano),
		sink.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("loggingrepo upsert sink: %w", err)
	}
	return nil
}

func (r *Repo) DeleteSink(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM log_sinks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("loggingrepo delete sink: %w", err)
	}
	return nil
}

func (r *Repo) GetRoutes(ctx context.Context) (RouteConfig, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, routes_json, updated_at
		   FROM log_route_configs
		  WHERE id = ?
		  LIMIT 1`,
		routeConfigID,
	)
	config, err := scanRouteConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC()
		config = &RouteConfig{ID: routeConfigID, RoutesJSON: "[]", UpdatedAt: now}
		if err := r.UpsertRoutes(ctx, *config); err != nil {
			return RouteConfig{}, err
		}
		return *config, nil
	}
	if err != nil {
		return RouteConfig{}, fmt.Errorf("loggingrepo get routes: %w", err)
	}
	return *config, nil
}

func (r *Repo) UpsertRoutes(ctx context.Context, config RouteConfig) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO log_route_configs (id, routes_json, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   routes_json = excluded.routes_json,
		   updated_at = excluded.updated_at`,
		routeConfigID,
		config.RoutesJSON,
		config.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("loggingrepo upsert routes: %w", err)
	}
	return nil
}

func (r *Repo) ListStatuses(ctx context.Context) ([]DeliveryStatus, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT sink_id, queue_depth, dropped_events, total_delivered, total_failed, consecutive_failures, last_attempted_at, last_delivered_at, last_error, updated_at
		   FROM log_delivery_status
		  ORDER BY sink_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("loggingrepo list statuses: %w", err)
	}
	defer rows.Close()

	var statuses []DeliveryStatus
	for rows.Next() {
		status, err := scanDeliveryStatus(rows)
		if err != nil {
			return nil, fmt.Errorf("loggingrepo list statuses scan: %w", err)
		}
		statuses = append(statuses, *status)
	}
	return statuses, rows.Err()
}

func (r *Repo) FindStatusBySinkID(ctx context.Context, sinkID string) (*DeliveryStatus, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT sink_id, queue_depth, dropped_events, total_delivered, total_failed, consecutive_failures, last_attempted_at, last_delivered_at, last_error, updated_at
		   FROM log_delivery_status
		  WHERE sink_id = ?
		  LIMIT 1`,
		sinkID,
	)
	status, err := scanDeliveryStatus(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loggingrepo find status: %w", err)
	}
	return status, nil
}

func (r *Repo) UpsertStatus(ctx context.Context, status DeliveryStatus) error {
	var attempted any
	if status.LastAttemptedAt != nil {
		attempted = status.LastAttemptedAt.UTC().Format(time.RFC3339Nano)
	}
	var delivered any
	if status.LastDeliveredAt != nil {
		delivered = status.LastDeliveredAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO log_delivery_status (sink_id, queue_depth, dropped_events, total_delivered, total_failed, consecutive_failures, last_attempted_at, last_delivered_at, last_error, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(sink_id) DO UPDATE SET
		   queue_depth = excluded.queue_depth,
		   dropped_events = excluded.dropped_events,
		   total_delivered = excluded.total_delivered,
		   total_failed = excluded.total_failed,
		   consecutive_failures = excluded.consecutive_failures,
		   last_attempted_at = excluded.last_attempted_at,
		   last_delivered_at = excluded.last_delivered_at,
		   last_error = excluded.last_error,
		   updated_at = excluded.updated_at`,
		status.SinkID,
		status.QueueDepth,
		status.DroppedEvents,
		status.TotalDelivered,
		status.TotalFailed,
		status.ConsecutiveFailures,
		attempted,
		delivered,
		status.LastError,
		status.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("loggingrepo upsert status: %w", err)
	}
	return nil
}

func (r *Repo) DeleteStatusBySinkID(ctx context.Context, sinkID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM log_delivery_status WHERE sink_id = ?`, sinkID); err != nil {
		return fmt.Errorf("loggingrepo delete status: %w", err)
	}
	return nil
}

func (r *Repo) InsertDeadLetter(ctx context.Context, deadLetter DeadLetter) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO log_delivery_dead_letters (id, sink_id, occurred_at, event_json, error_message, test_delivery, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		deadLetter.ID,
		deadLetter.SinkID,
		deadLetter.OccurredAt.UTC().Format(time.RFC3339Nano),
		deadLetter.EventJSON,
		deadLetter.ErrorMessage,
		deadLetter.TestDelivery,
		deadLetter.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("loggingrepo insert dead letter: %w", err)
	}
	return nil
}

func (r *Repo) ListRecentDeadLetters(ctx context.Context, limit int) ([]DeadLetter, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, sink_id, occurred_at, event_json, error_message, test_delivery, created_at
		   FROM log_delivery_dead_letters
		  ORDER BY created_at DESC, id DESC
		  LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("loggingrepo list dead letters: %w", err)
	}
	defer rows.Close()

	var deadLetters []DeadLetter
	for rows.Next() {
		deadLetter, err := scanDeadLetter(rows)
		if err != nil {
			return nil, fmt.Errorf("loggingrepo scan dead letter: %w", err)
		}
		deadLetters = append(deadLetters, *deadLetter)
	}
	return deadLetters, rows.Err()
}

func (r *Repo) ListDeadLetterCounts(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT sink_id, COUNT(*)
		   FROM log_delivery_dead_letters
		  GROUP BY sink_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("loggingrepo list dead letter counts: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var sinkID string
		var count int
		if err := rows.Scan(&sinkID, &count); err != nil {
			return nil, fmt.Errorf("loggingrepo scan dead letter count: %w", err)
		}
		counts[sinkID] = count
	}
	return counts, rows.Err()
}

func (r *Repo) PruneDeadLettersBySink(ctx context.Context, sinkID string, keep int) error {
	if keep <= 0 {
		return nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id
		   FROM log_delivery_dead_letters
		  WHERE sink_id = ?
		  ORDER BY created_at DESC, id DESC`,
		sinkID,
	)
	if err != nil {
		return fmt.Errorf("loggingrepo query dead letters for prune: %w", err)
	}
	defer rows.Close()

	var staleIDs []string
	index := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("loggingrepo scan dead letter prune row: %w", err)
		}
		if index >= keep {
			staleIDs = append(staleIDs, id)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("loggingrepo iterate dead letter prune rows: %w", err)
	}
	for _, id := range staleIDs {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM log_delivery_dead_letters WHERE id = ?`, id); err != nil {
			return fmt.Errorf("loggingrepo delete dead letter: %w", err)
		}
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSink(row scanner) (*Sink, error) {
	var sink Sink
	var enabled bool
	var createdAt string
	var updatedAt string
	if err := row.Scan(&sink.ID, &sink.Name, &sink.Type, &enabled, &sink.ConfigJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	sink.Enabled = enabled
	sink.CreatedAt = parseTimestamp(createdAt)
	sink.UpdatedAt = parseTimestamp(updatedAt)
	return &sink, nil
}

func scanRouteConfig(row scanner) (*RouteConfig, error) {
	var config RouteConfig
	var updatedAt string
	if err := row.Scan(&config.ID, &config.RoutesJSON, &updatedAt); err != nil {
		return nil, err
	}
	config.UpdatedAt = parseTimestamp(updatedAt)
	return &config, nil
}

func scanDeliveryStatus(row scanner) (*DeliveryStatus, error) {
	var status DeliveryStatus
	var attempted sql.NullString
	var delivered sql.NullString
	var updatedAt string
	if err := row.Scan(&status.SinkID, &status.QueueDepth, &status.DroppedEvents, &status.TotalDelivered, &status.TotalFailed, &status.ConsecutiveFailures, &attempted, &delivered, &status.LastError, &updatedAt); err != nil {
		return nil, err
	}
	if attempted.Valid {
		parsed := parseTimestamp(attempted.String)
		status.LastAttemptedAt = &parsed
	}
	if delivered.Valid {
		parsed := parseTimestamp(delivered.String)
		status.LastDeliveredAt = &parsed
	}
	status.UpdatedAt = parseTimestamp(updatedAt)
	return &status, nil
}

func scanDeadLetter(row scanner) (*DeadLetter, error) {
	var deadLetter DeadLetter
	var occurredAt string
	var createdAt string
	if err := row.Scan(&deadLetter.ID, &deadLetter.SinkID, &occurredAt, &deadLetter.EventJSON, &deadLetter.ErrorMessage, &deadLetter.TestDelivery, &createdAt); err != nil {
		return nil, err
	}
	deadLetter.OccurredAt = parseTimestamp(occurredAt)
	deadLetter.CreatedAt = parseTimestamp(createdAt)
	return &deadLetter, nil
}

func parseTimestamp(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
