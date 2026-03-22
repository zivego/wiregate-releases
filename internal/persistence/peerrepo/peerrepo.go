package peerrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Peer mirrors the peers table.
type Peer struct {
	ID              string
	AgentID         string
	PublicKey       string
	AssignedAddress string
	AllowedIPs      []string
	Status          string
	CreatedAt       time.Time
}

// Repo provides peer persistence operations.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Insert(ctx context.Context, peer Peer) error {
	var assignedAddress any
	if peer.AssignedAddress != "" {
		assignedAddress = peer.AssignedAddress
	}
	allowedIPsJSON, err := json.Marshal(peer.AllowedIPs)
	if err != nil {
		return fmt.Errorf("peerrepo insert marshal allowed ips: %w", err)
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO peers (id, agent_id, public_key, assigned_address, allowed_ips_json, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		peer.ID,
		peer.AgentID,
		peer.PublicKey,
		assignedAddress,
		string(allowedIPsJSON),
		peer.Status,
		peer.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("peerrepo insert: %w", err)
	}
	return nil
}

func (r *Repo) FindByAgentID(ctx context.Context, agentID string) (*Peer, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, agent_id, public_key, assigned_address, allowed_ips_json, status, created_at
		   FROM peers
		  WHERE agent_id = ?
		  LIMIT 1`,
		agentID,
	)
	peer, err := scanPeer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("peerrepo find by agent id: %w", err)
	}
	return peer, nil
}

func (r *Repo) FindByAgentIDs(ctx context.Context, agentIDs []string) (map[string]Peer, error) {
	if len(agentIDs) == 0 {
		return map[string]Peer{}, nil
	}
	placeholders := make([]string, 0, len(agentIDs))
	args := make([]any, 0, len(agentIDs))
	for idx, agentID := range agentIDs {
		if strings.TrimSpace(agentID) == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, agentID)
		_ = idx
	}
	if len(placeholders) == 0 {
		return map[string]Peer{}, nil
	}
	query := `SELECT id, agent_id, public_key, assigned_address, allowed_ips_json, status, created_at
		FROM peers
		WHERE agent_id IN (` + strings.Join(placeholders, ", ") + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("peerrepo find by agent ids: %w", err)
	}
	defer rows.Close()

	peers := make(map[string]Peer, len(agentIDs))
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, fmt.Errorf("peerrepo find by agent ids scan: %w", err)
		}
		peers[peer.AgentID] = *peer
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("peerrepo find by agent ids rows: %w", err)
	}
	return peers, nil
}

func (r *Repo) FindByPublicKey(ctx context.Context, publicKey string) (*Peer, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, agent_id, public_key, assigned_address, allowed_ips_json, status, created_at
		   FROM peers
		  WHERE public_key = ?
		  LIMIT 1`,
		publicKey,
	)
	peer, err := scanPeer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("peerrepo find by public key: %w", err)
	}
	return peer, nil
}

func (r *Repo) FindByID(ctx context.Context, id string) (*Peer, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, agent_id, public_key, assigned_address, allowed_ips_json, status, created_at
		   FROM peers
		  WHERE id = ?
		  LIMIT 1`,
		id,
	)
	peer, err := scanPeer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("peerrepo find by id: %w", err)
	}
	return peer, nil
}

func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Peer, error) {
	peers, _, err := r.ListPage(ctx, filter)
	return peers, err
}

func (r *Repo) ListPage(ctx context.Context, filter ListFilter) ([]Peer, bool, error) {
	query := `SELECT id, agent_id, public_key, assigned_address, allowed_ips_json, status, created_at FROM peers`
	var clauses []string
	var args []any
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.AgentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Query != "" {
		clauses = append(clauses, "public_key LIKE ?")
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
		return nil, false, fmt.Errorf("peerrepo list: %w", err)
	}
	defer rows.Close()

	var peers []Peer
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, false, fmt.Errorf("peerrepo list scan: %w", err)
		}
		peers = append(peers, *peer)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(peers) > limit
	if hasMore {
		peers = peers[:limit]
	}
	return peers, hasMore, nil
}

func (r *Repo) UpdateIntent(ctx context.Context, id string, allowedIPs []string, status string) error {
	allowedIPsJSON, err := json.Marshal(allowedIPs)
	if err != nil {
		return fmt.Errorf("peerrepo update intent marshal allowed ips: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE peers
		    SET allowed_ips_json = ?, status = ?
		  WHERE id = ?`,
		string(allowedIPsJSON),
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf("peerrepo update intent: %w", err)
	}
	return nil
}

func (r *Repo) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE peers
		    SET status = ?
		  WHERE id = ?`,
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf("peerrepo update status: %w", err)
	}
	return nil
}

// ClearAssignedAddress sets the peer's assigned_address to NULL, reclaiming the IP.
func (r *Repo) ClearAssignedAddress(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE peers SET assigned_address = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("peerrepo clear assigned address: %w", err)
	}
	return nil
}

func (r *Repo) UpdatePublicKeyAndStatus(ctx context.Context, id, publicKey, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE peers
		    SET public_key = ?, status = ?
		  WHERE id = ?`,
		publicKey,
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf("peerrepo update public key and status: %w", err)
	}
	return nil
}

// ListFilter limits peer inventory results.
type ListFilter struct {
	Status     string
	AgentID    string
	Query      string
	Limit      int
	CursorID   string
	CursorTime time.Time
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPeer(row scanner) (*Peer, error) {
	var peer Peer
	var assignedAddress sql.NullString
	var allowedIPsJSON string
	var createdAt string
	err := row.Scan(&peer.ID, &peer.AgentID, &peer.PublicKey, &assignedAddress, &allowedIPsJSON, &peer.Status, &createdAt)
	if err != nil {
		return nil, err
	}
	peer.AssignedAddress = assignedAddress.String
	if allowedIPsJSON != "" {
		_ = json.Unmarshal([]byte(allowedIPsJSON), &peer.AllowedIPs)
	}
	peer.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &peer, nil
}
