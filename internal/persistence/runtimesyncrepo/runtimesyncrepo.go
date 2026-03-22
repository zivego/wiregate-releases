package runtimesyncrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// State mirrors the runtime_sync_state table.
type State struct {
	ID               string
	PeerID           string
	DriftState       string
	LastObservedAt   time.Time
	LastReconciledAt *time.Time
	DetailsJSON      string
}

// Repo provides runtime sync state persistence operations.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Upsert(ctx context.Context, state State) error {
	var peerID any
	if state.PeerID != "" {
		peerID = state.PeerID
	}
	var lastReconciledAt any
	if state.LastReconciledAt != nil {
		lastReconciledAt = state.LastReconciledAt.UTC().Format(time.RFC3339Nano)
	}
	var details any
	if state.DetailsJSON != "" {
		details = state.DetailsJSON
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO runtime_sync_state (id, peer_id, drift_state, last_observed_at, last_reconciled_at, details)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   peer_id = excluded.peer_id,
		   drift_state = excluded.drift_state,
		   last_observed_at = excluded.last_observed_at,
		   last_reconciled_at = excluded.last_reconciled_at,
		   details = excluded.details`,
		state.ID,
		peerID,
		state.DriftState,
		state.LastObservedAt.UTC().Format(time.RFC3339Nano),
		lastReconciledAt,
		details,
	)
	if err != nil {
		return fmt.Errorf("runtimesyncrepo upsert: %w", err)
	}
	return nil
}

func (r *Repo) FindByPeerID(ctx context.Context, peerID string) (*State, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, peer_id, drift_state, last_observed_at, last_reconciled_at, details
		   FROM runtime_sync_state
		  WHERE peer_id = ?
		  LIMIT 1`,
		peerID,
	)
	state, err := scanState(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("runtimesyncrepo find by peer id: %w", err)
	}
	return state, nil
}

func (r *Repo) List(ctx context.Context) ([]State, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, peer_id, drift_state, last_observed_at, last_reconciled_at, details
		   FROM runtime_sync_state
		  ORDER BY last_observed_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("runtimesyncrepo list: %w", err)
	}
	defer rows.Close()

	var states []State
	for rows.Next() {
		state, err := scanState(rows)
		if err != nil {
			return nil, fmt.Errorf("runtimesyncrepo list scan: %w", err)
		}
		states = append(states, *state)
	}
	return states, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanState(row scanner) (*State, error) {
	var state State
	var peerID sql.NullString
	var lastObservedAt string
	var lastReconciledAt sql.NullString
	var details sql.NullString
	if err := row.Scan(&state.ID, &peerID, &state.DriftState, &lastObservedAt, &lastReconciledAt, &details); err != nil {
		return nil, err
	}
	state.PeerID = peerID.String
	state.LastObservedAt = parseTimestamp(lastObservedAt)
	if lastReconciledAt.Valid {
		parsed := parseTimestamp(lastReconciledAt.String)
		state.LastReconciledAt = &parsed
	}
	state.DetailsJSON = details.String
	return &state, nil
}

func parseTimestamp(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
