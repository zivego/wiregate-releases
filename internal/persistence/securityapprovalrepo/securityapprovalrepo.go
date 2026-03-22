package securityapprovalrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

type Approval struct {
	ID                 string
	Action             string
	ResourceType       string
	ResourceID         string
	RequestPayloadJSON string
	RequestedByUserID  string
	ApprovedByUserID   string
	RejectedByUserID   string
	Status             string
	CreatedAt          time.Time
	DecidedAt          *time.Time
}

type ListFilter struct {
	Status     string
	Limit      int
	CursorID   string
	CursorTime time.Time
}

type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Insert(ctx context.Context, approval Approval) error {
	var decidedAt any
	if approval.DecidedAt != nil {
		decidedAt = approval.DecidedAt.UTC().Format(time.RFC3339)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO security_approvals (
			id, action, resource_type, resource_id, request_payload_json, requested_by_user_id,
			approved_by_user_id, rejected_by_user_id, status, created_at, decided_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		approval.ID,
		approval.Action,
		approval.ResourceType,
		approval.ResourceID,
		approval.RequestPayloadJSON,
		approval.RequestedByUserID,
		nullString(approval.ApprovedByUserID),
		nullString(approval.RejectedByUserID),
		approval.Status,
		approval.CreatedAt.UTC().Format(time.RFC3339),
		decidedAt,
	)
	if err != nil {
		return fmt.Errorf("securityapprovalrepo insert: %w", err)
	}
	return nil
}

func (r *Repo) FindByID(ctx context.Context, id string) (*Approval, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, action, resource_type, resource_id, request_payload_json, requested_by_user_id, approved_by_user_id, rejected_by_user_id, status, created_at, decided_at
		   FROM security_approvals
		  WHERE id = ?
		  LIMIT 1`,
		id,
	)
	approval, err := scanApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("securityapprovalrepo find by id: %w", err)
	}
	return approval, nil
}

func (r *Repo) FindPendingByActionResource(ctx context.Context, action, resourceType, resourceID string) (*Approval, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, action, resource_type, resource_id, request_payload_json, requested_by_user_id, approved_by_user_id, rejected_by_user_id, status, created_at, decided_at
		   FROM security_approvals
		  WHERE action = ? AND resource_type = ? AND resource_id = ? AND status = ?
		  LIMIT 1`,
		action,
		resourceType,
		resourceID,
		"pending",
	)
	approval, err := scanApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("securityapprovalrepo find pending by action resource: %w", err)
	}
	return approval, nil
}

func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Approval, error) {
	approvals, _, err := r.ListPage(ctx, filter)
	return approvals, err
}

func (r *Repo) ListPage(ctx context.Context, filter ListFilter) ([]Approval, bool, error) {
	query := `SELECT id, action, resource_type, resource_id, request_payload_json, requested_by_user_id, approved_by_user_id, rejected_by_user_id, status, created_at, decided_at
		FROM security_approvals`
	var args []any
	var clauses []string
	hasWhere := false
	if strings.TrimSpace(filter.Status) != "" {
		query += ` WHERE status = ?`
		args = append(args, strings.TrimSpace(filter.Status))
		hasWhere = true
	}
	if strings.TrimSpace(filter.CursorID) != "" && !filter.CursorTime.IsZero() {
		cursorTime := filter.CursorTime.UTC().Format(time.RFC3339)
		clauses = append(clauses, `(created_at < ? OR (created_at = ? AND id < ?))`)
		args = append(args, cursorTime, cursorTime, strings.TrimSpace(filter.CursorID))
	}
	if len(clauses) > 0 {
		if !hasWhere {
			query += ` WHERE ` + strings.Join(clauses, ` AND `)
		} else {
			query += ` AND ` + strings.Join(clauses, ` AND `)
		}
	}
	query += ` ORDER BY created_at DESC, id DESC`
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query += ` LIMIT ?`
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("securityapprovalrepo list: %w", err)
	}
	defer rows.Close()

	var approvals []Approval
	for rows.Next() {
		approval, err := scanApproval(rows)
		if err != nil {
			return nil, false, fmt.Errorf("securityapprovalrepo list scan: %w", err)
		}
		approvals = append(approvals, *approval)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(approvals) > limit
	if hasMore {
		approvals = approvals[:limit]
	}
	return approvals, hasMore, nil
}

func (r *Repo) MarkApproved(ctx context.Context, id, approvedByUserID string, decidedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE security_approvals
		    SET status = ?, approved_by_user_id = ?, rejected_by_user_id = NULL, decided_at = ?
		  WHERE id = ? AND status = ?`,
		"approved",
		approvedByUserID,
		decidedAt.UTC().Format(time.RFC3339),
		id,
		"pending",
	)
	if err != nil {
		return fmt.Errorf("securityapprovalrepo mark approved: %w", err)
	}
	return nil
}

func (r *Repo) MarkRejected(ctx context.Context, id, rejectedByUserID string, decidedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE security_approvals
		    SET status = ?, approved_by_user_id = NULL, rejected_by_user_id = ?, decided_at = ?
		  WHERE id = ? AND status = ?`,
		"rejected",
		rejectedByUserID,
		decidedAt.UTC().Format(time.RFC3339),
		id,
		"pending",
	)
	if err != nil {
		return fmt.Errorf("securityapprovalrepo mark rejected: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanApproval(row scanner) (*Approval, error) {
	var approval Approval
	var approvedByUserID sql.NullString
	var rejectedByUserID sql.NullString
	var createdAt string
	var decidedAt sql.NullString
	if err := row.Scan(
		&approval.ID,
		&approval.Action,
		&approval.ResourceType,
		&approval.ResourceID,
		&approval.RequestPayloadJSON,
		&approval.RequestedByUserID,
		&approvedByUserID,
		&rejectedByUserID,
		&approval.Status,
		&createdAt,
		&decidedAt,
	); err != nil {
		return nil, err
	}
	approval.ApprovedByUserID = approvedByUserID.String
	approval.RejectedByUserID = rejectedByUserID.String
	approval.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if decidedAt.Valid {
		parsed, _ := time.Parse(time.RFC3339, decidedAt.String)
		approval.DecidedAt = &parsed
	}
	return &approval, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
