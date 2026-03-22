package sessionrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Session mirrors the sessions table.
type Session struct {
	ID            string // SHA-256 hash of the raw bearer token
	PublicID      string
	UserID        string
	Email         string
	Role          string
	AuthProvider  string
	IssuedAt      time.Time
	ExpiresAt     time.Time
	LastSeenAt    time.Time
	SourceIP      string
	UserAgent     string
	RevokedAt     *time.Time
	RevokedReason string
}

type ListFilter struct {
	UserID      string
	IdleCutoff  time.Time
	Limit       int
	CursorID    string
	CursorSeen  time.Time
	CursorIssue time.Time
}

// Repo provides session persistence operations.
type Repo struct {
	db *persistdb.Handle
}

// New creates a Repo backed by db.
func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

// Insert persists a new session.
func (r *Repo) Insert(ctx context.Context, s Session) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (
			id, public_id, user_id, auth_provider, issued_at, expires_at, last_seen_at, source_ip, user_agent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID,
		s.PublicID,
		s.UserID,
		normalizeAuthProvider(s.AuthProvider),
		s.IssuedAt.UTC().Format(time.RFC3339),
		s.ExpiresAt.UTC().Format(time.RFC3339),
		s.LastSeenAt.UTC().Format(time.RFC3339),
		s.SourceIP,
		s.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("sessionrepo insert: %w", err)
	}
	return nil
}

// FindActive returns the session matching tokenHash that is not expired or revoked.
func (r *Repo) FindActive(ctx context.Context, tokenHash string, idleCutoff time.Time) (*Session, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	row := r.db.QueryRowContext(ctx,
		`SELECT s.id, s.public_id, s.user_id, u.email, u.role, s.auth_provider, s.issued_at, s.expires_at, s.last_seen_at, s.source_ip, s.user_agent, s.revoked_at, s.revoked_reason
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.id = ? AND s.expires_at > ? AND s.revoked_at IS NULL AND s.last_seen_at > ?
		  LIMIT 1`,
		tokenHash, now, idleCutoff.UTC().Format(time.RFC3339),
	)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sessionrepo find active: %w", err)
	}
	return s, nil
}

// Revoke sets revoked_at on the session identified by tokenHash.
func (r *Repo) Revoke(ctx context.Context, tokenHash, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ?, revoked_reason = ? WHERE id = ?`,
		now, normalizeRevokedReason(reason), tokenHash,
	)
	if err != nil {
		return fmt.Errorf("sessionrepo revoke: %w", err)
	}
	return nil
}

// RevokeAllForUser revokes all active sessions for userID.
func (r *Repo) RevokeAllForUser(ctx context.Context, userID, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions
		    SET revoked_at = ?, revoked_reason = ?
		  WHERE user_id = ?
		    AND revoked_at IS NULL`,
		now, normalizeRevokedReason(reason), userID,
	)
	if err != nil {
		return fmt.Errorf("sessionrepo revoke all for user: %w", err)
	}
	return nil
}

func (r *Repo) Touch(ctx context.Context, tokenHash string, seenAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions
		    SET last_seen_at = ?
		  WHERE id = ?
		    AND revoked_at IS NULL`,
		seenAt.UTC().Format(time.RFC3339),
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("sessionrepo touch: %w", err)
	}
	return nil
}

func (r *Repo) FindByPublicID(ctx context.Context, publicID string) (*Session, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT s.id, s.public_id, s.user_id, u.email, u.role, s.auth_provider, s.issued_at, s.expires_at, s.last_seen_at, s.source_ip, s.user_agent, s.revoked_at, s.revoked_reason
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.public_id = ?
		  LIMIT 1`,
		publicID,
	)
	session, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sessionrepo find by public id: %w", err)
	}
	return session, nil
}

func (r *Repo) List(ctx context.Context, filter ListFilter) ([]Session, error) {
	sessions, _, err := r.ListPage(ctx, filter)
	return sessions, err
}

func (r *Repo) ListPage(ctx context.Context, filter ListFilter) ([]Session, bool, error) {
	query := `SELECT s.id, s.public_id, s.user_id, u.email, u.role, s.auth_provider, s.issued_at, s.expires_at, s.last_seen_at, s.source_ip, s.user_agent, s.revoked_at, s.revoked_reason
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.revoked_at IS NULL
		  AND s.expires_at > ?
		  AND s.last_seen_at > ?`
	var clauses []string
	idleCutoff := filter.IdleCutoff
	if idleCutoff.IsZero() {
		idleCutoff = time.Now().UTC().Add(-30 * time.Minute)
	}
	args := []any{
		time.Now().UTC().Format(time.RFC3339),
		idleCutoff.UTC().Format(time.RFC3339),
	}
	if strings.TrimSpace(filter.UserID) != "" {
		clauses = append(clauses, "s.user_id = ?")
		args = append(args, strings.TrimSpace(filter.UserID))
	}
	if strings.TrimSpace(filter.CursorID) != "" && !filter.CursorSeen.IsZero() && !filter.CursorIssue.IsZero() {
		cursorSeen := filter.CursorSeen.UTC().Format(time.RFC3339)
		cursorIssued := filter.CursorIssue.UTC().Format(time.RFC3339)
		clauses = append(clauses, `(s.last_seen_at < ? OR (s.last_seen_at = ? AND s.issued_at < ?) OR (s.last_seen_at = ? AND s.issued_at = ? AND s.public_id < ?))`)
		args = append(args, cursorSeen, cursorSeen, cursorIssued, cursorSeen, cursorIssued, strings.TrimSpace(filter.CursorID))
	}
	if len(clauses) > 0 {
		query += " AND " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY s.last_seen_at DESC, s.issued_at DESC, s.public_id DESC"
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query += " LIMIT ?"
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("sessionrepo list: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, false, fmt.Errorf("sessionrepo list scan: %w", err)
		}
		sessions = append(sessions, *session)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(sessions) > limit
	if hasMore {
		sessions = sessions[:limit]
	}
	return sessions, hasMore, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(row scanner) (*Session, error) {
	var session Session
	var authProvider string
	var issuedAt string
	var expiresAt string
	var lastSeenAt string
	var sourceIP string
	var userAgent string
	var revokedAt sql.NullString
	var revokedReason string
	if err := row.Scan(
		&session.ID,
		&session.PublicID,
		&session.UserID,
		&session.Email,
		&session.Role,
		&authProvider,
		&issuedAt,
		&expiresAt,
		&lastSeenAt,
		&sourceIP,
		&userAgent,
		&revokedAt,
		&revokedReason,
	); err != nil {
		return nil, err
	}
	session.AuthProvider = normalizeAuthProvider(authProvider)
	session.IssuedAt, _ = time.Parse(time.RFC3339, issuedAt)
	session.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	session.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeenAt)
	session.SourceIP = sourceIP
	session.UserAgent = userAgent
	if revokedAt.Valid {
		parsed, _ := time.Parse(time.RFC3339, revokedAt.String)
		session.RevokedAt = &parsed
	}
	session.RevokedReason = revokedReason
	return &session, nil
}

func normalizeAuthProvider(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "oidc":
		return "oidc"
	default:
		return "password"
	}
}

func normalizeRevokedReason(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "revoked"
	}
	return trimmed
}
