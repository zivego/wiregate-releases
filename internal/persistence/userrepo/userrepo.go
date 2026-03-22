package userrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// User mirrors the users table.
type User struct {
	ID                 string
	Email              string
	Role               string
	PasswordHash       string
	MustChangePassword bool
	MFATOTPSecret      string
	MFATOTPEnabled     bool
	ThemePreference    string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Repo provides user persistence operations.
type Repo struct {
	db *persistdb.Handle
}

// New creates a Repo backed by db.
func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

// FindByEmail returns the user with the given email, or nil if not found.
func (r *Repo) FindByEmail(ctx context.Context, email string) (*User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, email, role, password_hash, must_change_password, mfa_totp_secret, mfa_totp_enabled, theme_preference, created_at, updated_at
		   FROM users WHERE email = ? LIMIT 1`,
		email,
	)
	u := &User{}
	var createdAt, updatedAt string
	err := row.Scan(&u.ID, &u.Email, &u.Role, &u.PasswordHash, &u.MustChangePassword, &u.MFATOTPSecret, &u.MFATOTPEnabled, &u.ThemePreference, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("userrepo find by email: %w", err)
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return u, nil
}

// Insert adds a new user row.
func (r *Repo) Insert(ctx context.Context, u User) error {
	now := time.Now().UTC().Format(time.RFC3339)
	themePreference := normalizeThemePreference(u.ThemePreference)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, email, role, password_hash, must_change_password, mfa_totp_secret, mfa_totp_enabled, theme_preference, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.Role, u.PasswordHash, u.MustChangePassword, strings.TrimSpace(u.MFATOTPSecret), u.MFATOTPEnabled, themePreference, now, now,
	)
	if err != nil {
		return fmt.Errorf("userrepo insert: %w", err)
	}
	return nil
}

// FindByID returns the user with the given id, or nil if not found.
func (r *Repo) FindByID(ctx context.Context, id string) (*User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, email, role, password_hash, must_change_password, mfa_totp_secret, mfa_totp_enabled, theme_preference, created_at, updated_at
		   FROM users WHERE id = ? LIMIT 1`,
		id,
	)
	u := &User{}
	var createdAt, updatedAt string
	err := row.Scan(&u.ID, &u.Email, &u.Role, &u.PasswordHash, &u.MustChangePassword, &u.MFATOTPSecret, &u.MFATOTPEnabled, &u.ThemePreference, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("userrepo find by id: %w", err)
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return u, nil
}

// List returns all users ordered by created_at.
func (r *Repo) List(ctx context.Context) ([]User, error) {
	users, _, err := r.ListPage(ctx, ListFilter{})
	return users, err
}

func (r *Repo) ListPage(ctx context.Context, filter ListFilter) ([]User, bool, error) {
	query := `SELECT id, email, role, must_change_password, mfa_totp_secret, mfa_totp_enabled, theme_preference, created_at, updated_at FROM users`
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
		return nil, false, fmt.Errorf("userrepo list: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var createdAt, updatedAt string
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.MustChangePassword, &u.MFATOTPSecret, &u.MFATOTPEnabled, &u.ThemePreference, &createdAt, &updatedAt); err != nil {
			return nil, false, fmt.Errorf("userrepo list scan: %w", err)
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}
	return users, hasMore, nil
}

// Delete removes a user by id.
func (r *Repo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("userrepo delete: %w", err)
	}
	return nil
}

// UpdateThemePreference stores the UI theme preference for a user.
func (r *Repo) UpdateThemePreference(ctx context.Context, id, themePreference string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET theme_preference = ?, updated_at = ? WHERE id = ?`,
		normalizeThemePreference(themePreference), now, id,
	)
	if err != nil {
		return fmt.Errorf("userrepo update theme preference: %w", err)
	}
	return nil
}

// UpdateProfile stores admin-editable identity fields for a user.
func (r *Repo) UpdateProfile(ctx context.Context, id, email, role string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET email = ?, role = ?, updated_at = ? WHERE id = ?`,
		email, role, now, id,
	)
	if err != nil {
		return fmt.Errorf("userrepo update profile: %w", err)
	}
	return nil
}

// UpdatePasswordHash stores a new bcrypt password hash for a user.
func (r *Repo) UpdatePasswordHash(ctx context.Context, id, passwordHash string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, now, id,
	)
	if err != nil {
		return fmt.Errorf("userrepo update password hash: %w", err)
	}
	return nil
}

// UpdatePasswordHashAndMustChangePassword stores a new bcrypt password hash and forced-change flag.
func (r *Repo) UpdatePasswordHashAndMustChangePassword(ctx context.Context, id, passwordHash string, mustChangePassword bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, must_change_password = ?, updated_at = ? WHERE id = ?`,
		passwordHash, mustChangePassword, now, id,
	)
	if err != nil {
		return fmt.Errorf("userrepo update password hash and force-change: %w", err)
	}
	return nil
}

// SetMFATOTP stores TOTP secret and enabled flag for a user.
func (r *Repo) SetMFATOTP(ctx context.Context, id, secret string, enabled bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET mfa_totp_secret = ?, mfa_totp_enabled = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(secret), enabled, now, id,
	)
	if err != nil {
		return fmt.Errorf("userrepo set mfa totp: %w", err)
	}
	return nil
}

// ListAdminEmails returns email addresses of all admin users.
func (r *Repo) ListAdminEmails(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT email FROM users WHERE role = 'admin' ORDER BY email`)
	if err != nil {
		return nil, fmt.Errorf("userrepo list admin emails: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("userrepo list admin emails scan: %w", err)
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

// GetEmailByUserID returns the email for a single user, or empty string if not found.
func (r *Repo) GetEmailByUserID(ctx context.Context, userID string) (string, error) {
	var email string
	err := r.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = ? LIMIT 1`, userID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("userrepo get email by user id: %w", err)
	}
	return email, nil
}

// Count returns the total number of users.
func (r *Repo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("userrepo count: %w", err)
	}
	return n, nil
}

func normalizeThemePreference(themePreference string) string {
	if themePreference == "dark" {
		return "dark"
	}
	return "light"
}

type ListFilter struct {
	Limit      int
	CursorID   string
	CursorTime time.Time
}
