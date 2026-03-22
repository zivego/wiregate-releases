// Package ipamrepo provides persistence for IPAM pools and reservations.
package ipamrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Pool represents an IPAM address pool.
type Pool struct {
	ID          string
	CIDR        string
	Description string
	IsIPv6      bool
	Gateway     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Reservation represents a single address reservation within a pool.
type Reservation struct {
	ID         string
	PoolID     string
	Address    string
	PeerID     string
	Label      string
	ReservedAt time.Time
}

// Repo manages IPAM pool and reservation records.
type Repo struct {
	db *persistdb.Handle
}

// New creates a new IPAM repository.
func New(handle *persistdb.Handle) *Repo {
	return &Repo{db: handle}
}

// InsertPool creates a new address pool.
func (r *Repo) InsertPool(ctx context.Context, pool Pool) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO ipam_pools (id, cidr, description, is_ipv6, gateway, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		pool.ID, pool.CIDR, pool.Description, pool.IsIPv6, pool.Gateway,
		pool.CreatedAt.UTC().Format(time.RFC3339),
		pool.UpdatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("ipamrepo insert pool: %w", err)
	}
	return nil
}

// FindPoolByID returns a pool by ID.
func (r *Repo) FindPoolByID(ctx context.Context, id string) (*Pool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, cidr, description, is_ipv6, gateway, created_at, updated_at
		   FROM ipam_pools WHERE id = ?`, id)
	pool, err := scanPool(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ipamrepo find pool by id: %w", err)
	}
	return pool, nil
}

// FindPoolByCIDR returns a pool by CIDR.
func (r *Repo) FindPoolByCIDR(ctx context.Context, cidr string) (*Pool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, cidr, description, is_ipv6, gateway, created_at, updated_at
		   FROM ipam_pools WHERE cidr = ?`, cidr)
	pool, err := scanPool(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ipamrepo find pool by cidr: %w", err)
	}
	return pool, nil
}

// ListPools returns all address pools.
func (r *Repo) ListPools(ctx context.Context) ([]Pool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, cidr, description, is_ipv6, gateway, created_at, updated_at
		   FROM ipam_pools ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("ipamrepo list pools: %w", err)
	}
	defer rows.Close()

	var pools []Pool
	for rows.Next() {
		pool, err := scanPool(rows)
		if err != nil {
			return nil, fmt.Errorf("ipamrepo list pools scan: %w", err)
		}
		pools = append(pools, *pool)
	}
	return pools, rows.Err()
}

// DeletePool deletes a pool by ID (cascades to reservations).
func (r *Repo) DeletePool(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM ipam_pools WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("ipamrepo delete pool: %w", err)
	}
	return nil
}

// InsertReservation creates a new address reservation.
func (r *Repo) InsertReservation(ctx context.Context, res Reservation) error {
	var peerID any
	if res.PeerID != "" {
		peerID = res.PeerID
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO ipam_reservations (id, pool_id, address, peer_id, label, reserved_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		res.ID, res.PoolID, res.Address, peerID, res.Label,
		res.ReservedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("ipamrepo insert reservation: %w", err)
	}
	return nil
}

// ListReservations returns all reservations for a given pool.
func (r *Repo) ListReservations(ctx context.Context, poolID string) ([]Reservation, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, pool_id, address, peer_id, label, reserved_at
		   FROM ipam_reservations WHERE pool_id = ? ORDER BY reserved_at ASC`, poolID)
	if err != nil {
		return nil, fmt.Errorf("ipamrepo list reservations: %w", err)
	}
	defer rows.Close()

	var reservations []Reservation
	for rows.Next() {
		res, err := scanReservation(rows)
		if err != nil {
			return nil, fmt.Errorf("ipamrepo list reservations scan: %w", err)
		}
		reservations = append(reservations, *res)
	}
	return reservations, rows.Err()
}

// DeleteReservation removes a reservation by ID.
func (r *Repo) DeleteReservation(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM ipam_reservations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("ipamrepo delete reservation: %w", err)
	}
	return nil
}

// DeleteReservationByPeerID removes a reservation by peer ID.
func (r *Repo) DeleteReservationByPeerID(ctx context.Context, peerID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM ipam_reservations WHERE peer_id = ?`, peerID)
	if err != nil {
		return fmt.Errorf("ipamrepo delete reservation by peer: %w", err)
	}
	return nil
}

// CountByPool returns the number of reservations in a pool.
func (r *Repo) CountByPool(ctx context.Context, poolID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ipam_reservations WHERE pool_id = ?`, poolID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ipamrepo count by pool: %w", err)
	}
	return count, nil
}

// PoolUsage returns the allocated address set for a pool.
func (r *Repo) PoolUsage(ctx context.Context, poolID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT address FROM ipam_reservations WHERE pool_id = ?`, poolID)
	if err != nil {
		return nil, fmt.Errorf("ipamrepo pool usage: %w", err)
	}
	defer rows.Close()

	var addrs []string
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return nil, fmt.Errorf("ipamrepo pool usage scan: %w", err)
		}
		addrs = append(addrs, addr)
	}
	return addrs, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPool(row scanner) (*Pool, error) {
	var p Pool
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.CIDR, &p.Description, &p.IsIPv6, &p.Gateway, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &p, nil
}

func scanReservation(row scanner) (*Reservation, error) {
	var r Reservation
	var peerID sql.NullString
	var reservedAt string
	err := row.Scan(&r.ID, &r.PoolID, &r.Address, &peerID, &r.Label, &reservedAt)
	if err != nil {
		return nil, err
	}
	r.PeerID = peerID.String
	r.ReservedAt, _ = time.Parse(time.RFC3339, reservedAt)
	return &r, nil
}
