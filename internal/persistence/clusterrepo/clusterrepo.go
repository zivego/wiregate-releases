package clusterrepo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

type Lease struct {
	LeaseName      string
	LeaderID       string
	LeaseExpiresAt time.Time
	UpdatedAt      time.Time
}

type Node struct {
	InstanceID      string
	IsLeader        bool
	LeaderID        string
	LastHeartbeatAt time.Time
	LeaseExpiresAt  *time.Time
	UpdatedAt       time.Time
}

type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) TryAcquireLease(ctx context.Context, leaseName, instanceID string, now time.Time, ttl time.Duration) (bool, Lease, error) {
	now = now.UTC()
	expiresAt := now.Add(ttl).Format(time.RFC3339)
	nowValue := now.Format(time.RFC3339)

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO cluster_leases (lease_name, leader_id, lease_expires_at, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(lease_name) DO UPDATE SET
			leader_id = excluded.leader_id,
			lease_expires_at = excluded.lease_expires_at,
			updated_at = excluded.updated_at
		 WHERE cluster_leases.lease_expires_at < ? OR cluster_leases.leader_id = excluded.leader_id`,
		strings.TrimSpace(leaseName),
		strings.TrimSpace(instanceID),
		expiresAt,
		nowValue,
		nowValue,
	)
	if err != nil {
		return false, Lease{}, fmt.Errorf("clusterrepo try acquire lease: %w", err)
	}
	rows, _ := result.RowsAffected()

	lease, err := r.GetLease(ctx, leaseName)
	if err != nil {
		return false, Lease{}, err
	}
	if lease == nil {
		return rows > 0, Lease{}, nil
	}
	return rows > 0 && lease.LeaderID == instanceID, *lease, nil
}

func (r *Repo) GetLease(ctx context.Context, leaseName string) (*Lease, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT lease_name, leader_id, lease_expires_at, updated_at
		   FROM cluster_leases
		  WHERE lease_name = ?
		  LIMIT 1`,
		strings.TrimSpace(leaseName),
	)
	var (
		lease        Lease
		expiresAtRaw string
		updatedAtRaw string
	)
	if err := row.Scan(&lease.LeaseName, &lease.LeaderID, &expiresAtRaw, &updatedAtRaw); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("clusterrepo get lease: %w", err)
	}
	lease.LeaseExpiresAt = parseTime(expiresAtRaw)
	lease.UpdatedAt = parseTime(updatedAtRaw)
	return &lease, nil
}

func (r *Repo) UpsertNode(ctx context.Context, node Node) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var leaseExpires any
	if node.LeaseExpiresAt != nil {
		leaseExpires = node.LeaseExpiresAt.UTC().Format(time.RFC3339)
	} else {
		leaseExpires = ""
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cluster_nodes (instance_id, is_leader, leader_id, last_heartbeat_at, lease_expires_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(instance_id) DO UPDATE SET
			is_leader = excluded.is_leader,
			leader_id = excluded.leader_id,
			last_heartbeat_at = excluded.last_heartbeat_at,
			lease_expires_at = excluded.lease_expires_at,
			updated_at = excluded.updated_at`,
		node.InstanceID,
		node.IsLeader,
		node.LeaderID,
		node.LastHeartbeatAt.UTC().Format(time.RFC3339),
		leaseExpires,
		now,
	)
	if err != nil {
		return fmt.Errorf("clusterrepo upsert node: %w", err)
	}
	return nil
}

func (r *Repo) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT instance_id, is_leader, leader_id, last_heartbeat_at, lease_expires_at, updated_at
		   FROM cluster_nodes
		  ORDER BY instance_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("clusterrepo list nodes: %w", err)
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var (
			node         Node
			heartbeatRaw string
			leaseRaw     string
			updatedRaw   string
		)
		if err := rows.Scan(&node.InstanceID, &node.IsLeader, &node.LeaderID, &heartbeatRaw, &leaseRaw, &updatedRaw); err != nil {
			return nil, fmt.Errorf("clusterrepo list nodes scan: %w", err)
		}
		node.LastHeartbeatAt = parseTime(heartbeatRaw)
		if strings.TrimSpace(leaseRaw) != "" {
			parsed := parseTime(leaseRaw)
			node.LeaseExpiresAt = &parsed
		}
		node.UpdatedAt = parseTime(updatedRaw)
		out = append(out, node)
	}
	return out, rows.Err()
}

func parseTime(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
