package enrollmenttokenrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Token mirrors the enrollment_tokens table.
type Token struct {
	ID              string
	TokenHash       string
	Model           string
	Scope           string
	Status          string
	BoundIdentity   string
	ExpiresAt       time.Time
	UsedAt          *time.Time
	RevokedAt       *time.Time
	CreatedByUserID string
	CreatedAt       time.Time
}

// Repo provides enrollment token persistence operations.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

// Insert persists a newly-issued token.
func (r *Repo) Insert(ctx context.Context, token Token) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO enrollment_tokens (
			id, token_hash, model, scope, status, bound_identity, expires_at, created_by_user_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token.ID,
		token.TokenHash,
		token.Model,
		token.Scope,
		token.Status,
		token.BoundIdentity,
		token.ExpiresAt.UTC().Format(time.RFC3339),
		token.CreatedByUserID,
		token.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("enrollmenttokenrepo insert: %w", err)
	}
	return nil
}

// FindByHash returns one token by hashed raw token or nil when absent.
func (r *Repo) FindByHash(ctx context.Context, tokenHash string) (*Token, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, token_hash, model, scope, status, bound_identity, expires_at, used_at, revoked_at, created_by_user_id, created_at
		   FROM enrollment_tokens
		  WHERE token_hash = ?
		  LIMIT 1`,
		tokenHash,
	)

	token, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("enrollmenttokenrepo find by hash: %w", err)
	}
	return token, nil
}

// FindByID returns one token by ID or nil when absent.
func (r *Repo) FindByID(ctx context.Context, id string) (*Token, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, token_hash, model, scope, status, bound_identity, expires_at, used_at, revoked_at, created_by_user_id, created_at
		   FROM enrollment_tokens
		  WHERE id = ?
		  LIMIT 1`,
		id,
	)

	token, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("enrollmenttokenrepo find by id: %w", err)
	}
	return token, nil
}

// List returns tokens ordered from newest to oldest.
func (r *Repo) List(ctx context.Context) ([]Token, error) {
	tokens, _, err := r.ListPage(ctx, ListFilter{})
	return tokens, err
}

func (r *Repo) ListPage(ctx context.Context, filter ListFilter) ([]Token, bool, error) {
	query := `SELECT id, token_hash, model, scope, status, bound_identity, expires_at, used_at, revoked_at, created_by_user_id, created_at
		FROM enrollment_tokens`
	var clauses []string
	var args []any
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
		return nil, false, fmt.Errorf("enrollmenttokenrepo list: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, false, fmt.Errorf("enrollmenttokenrepo list scan: %w", err)
		}
		tokens = append(tokens, *token)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(tokens) > limit
	if hasMore {
		tokens = tokens[:limit]
	}
	return tokens, hasMore, nil
}

// Revoke marks a token as revoked.
func (r *Repo) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE enrollment_tokens
		    SET status = ?, revoked_at = ?
		  WHERE id = ?`,
		"revoked",
		revokedAt.UTC().Format(time.RFC3339),
		id,
	)
	if err != nil {
		return fmt.Errorf("enrollmenttokenrepo revoke: %w", err)
	}
	return nil
}

// MarkUsed marks a token as consumed by a successful enrollment.
func (r *Repo) MarkUsed(ctx context.Context, id string, usedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE enrollment_tokens
		    SET status = ?, used_at = ?
		  WHERE id = ?`,
		"used",
		usedAt.UTC().Format(time.RFC3339),
		id,
	)
	if err != nil {
		return fmt.Errorf("enrollmenttokenrepo mark used: %w", err)
	}
	return nil
}

// BindPolicies links one enrollment token to pre-authorized access policies.
func (r *Repo) BindPolicies(ctx context.Context, tokenID string, policyIDs []string, createdAt time.Time) error {
	for _, policyID := range policyIDs {
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO enrollment_token_policy_bindings (token_id, access_policy_id, created_at)
			 VALUES (?, ?, ?)`,
			tokenID,
			policyID,
			createdAt.UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("enrollmenttokenrepo bind policies: %w", err)
		}
	}
	return nil
}

// ListPolicyIDs returns policy IDs bound to one enrollment token.
func (r *Repo) ListPolicyIDs(ctx context.Context, tokenID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT access_policy_id
		   FROM enrollment_token_policy_bindings
		  WHERE token_id = ?
		  ORDER BY access_policy_id ASC`,
		tokenID,
	)
	if err != nil {
		return nil, fmt.Errorf("enrollmenttokenrepo list policy ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("enrollmenttokenrepo list policy ids scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

type ListFilter struct {
	Limit      int
	CursorID   string
	CursorTime time.Time
}

func scanToken(row scanner) (*Token, error) {
	var token Token
	var expiresAt string
	var usedAt sql.NullString
	var revokedAt sql.NullString
	var createdAt string

	err := row.Scan(
		&token.ID,
		&token.TokenHash,
		&token.Model,
		&token.Scope,
		&token.Status,
		&token.BoundIdentity,
		&expiresAt,
		&usedAt,
		&revokedAt,
		&token.CreatedByUserID,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	token.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	token.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if usedAt.Valid {
		parsed, _ := time.Parse(time.RFC3339, usedAt.String)
		token.UsedAt = &parsed
	}
	if revokedAt.Valid {
		parsed, _ := time.Parse(time.RFC3339, revokedAt.String)
		token.RevokedAt = &parsed
	}

	return &token, nil
}
