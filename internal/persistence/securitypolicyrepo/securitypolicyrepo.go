package securitypolicyrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

const singletonID = "default"

type Policy struct {
	ID                  string
	RequiredAdminAMR    string
	RequiredAdminACR    string
	DualApprovalEnabled bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Get(ctx context.Context) (*Policy, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, required_admin_amr, required_admin_acr, dual_approval_enabled, created_at, updated_at
		   FROM security_policies
		  WHERE id = ?
		  LIMIT 1`,
		singletonID,
	)
	policy, err := scanPolicy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("securitypolicyrepo get: %w", err)
	}
	return policy, nil
}

func (r *Repo) Upsert(ctx context.Context, policy Policy) error {
	now := policy.UpdatedAt.UTC().Format(time.RFC3339)
	createdAt := policy.CreatedAt.UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO security_policies (id, required_admin_amr, required_admin_acr, dual_approval_enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   required_admin_amr = excluded.required_admin_amr,
		   required_admin_acr = excluded.required_admin_acr,
		   dual_approval_enabled = excluded.dual_approval_enabled,
		   updated_at = excluded.updated_at`,
		singletonID,
		policy.RequiredAdminAMR,
		policy.RequiredAdminACR,
		policy.DualApprovalEnabled,
		createdAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("securitypolicyrepo upsert: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPolicy(row scanner) (*Policy, error) {
	var policy Policy
	var dualApprovalEnabled bool
	var createdAt string
	var updatedAt string
	if err := row.Scan(&policy.ID, &policy.RequiredAdminAMR, &policy.RequiredAdminACR, &dualApprovalEnabled, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	policy.DualApprovalEnabled = dualApprovalEnabled
	policy.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	policy.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &policy, nil
}
