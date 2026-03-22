package serviceaccountrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

type ServiceAccount struct {
	ID         string
	Name       string
	Role       string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	LastUsedAt *time.Time
}

type APIKey struct {
	ID               string
	ServiceAccountID string
	Name             string
	KeyPrefix        string
	TokenHash        string
	Status           string
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	RevokedAt        *time.Time
	LastUsedAt       *time.Time
}

type KeyWithAccount struct {
	Key           APIKey
	AccountName   string
	AccountRole   string
	AccountStatus string
}

type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) InsertAccount(ctx context.Context, account ServiceAccount) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO service_accounts (id, name, role, status, created_at, updated_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		account.ID,
		strings.TrimSpace(account.Name),
		strings.TrimSpace(account.Role),
		normalizeStatus(account.Status),
		now,
		now,
		nil,
	)
	if err != nil {
		return fmt.Errorf("serviceaccountrepo insert account: %w", err)
	}
	return nil
}

func (r *Repo) ListAccounts(ctx context.Context) ([]ServiceAccount, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, role, status, created_at, updated_at, last_used_at
		   FROM service_accounts
		  ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("serviceaccountrepo list accounts: %w", err)
	}
	defer rows.Close()

	var out []ServiceAccount
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *account)
	}
	return out, rows.Err()
}

func (r *Repo) FindAccountByID(ctx context.Context, id string) (*ServiceAccount, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, role, status, created_at, updated_at, last_used_at
		   FROM service_accounts
		  WHERE id = ?
		  LIMIT 1`,
		id,
	)
	account, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("serviceaccountrepo find account: %w", err)
	}
	return account, nil
}

func (r *Repo) InsertKey(ctx context.Context, key APIKey) error {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	var expiresAt any
	if key.ExpiresAt != nil {
		expiresAt = key.ExpiresAt.UTC().Format(time.RFC3339)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO service_account_keys (
			id, service_account_id, name, key_prefix, token_hash, status, expires_at, created_at, revoked_at, last_used_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key.ID,
		key.ServiceAccountID,
		strings.TrimSpace(key.Name),
		strings.TrimSpace(key.KeyPrefix),
		strings.TrimSpace(key.TokenHash),
		normalizeStatus(key.Status),
		expiresAt,
		createdAt,
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("serviceaccountrepo insert key: %w", err)
	}
	return nil
}

func (r *Repo) ListKeysByAccount(ctx context.Context, accountID string) ([]APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, service_account_id, name, key_prefix, token_hash, status, expires_at, created_at, revoked_at, last_used_at
		   FROM service_account_keys
		  WHERE service_account_id = ?
		  ORDER BY created_at DESC, id DESC`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("serviceaccountrepo list keys: %w", err)
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		key, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *key)
	}
	return out, rows.Err()
}

func (r *Repo) FindActiveKeyByHash(ctx context.Context, tokenHash string, now time.Time) (*KeyWithAccount, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT
			k.id, k.service_account_id, k.name, k.key_prefix, k.token_hash, k.status, k.expires_at, k.created_at, k.revoked_at, k.last_used_at,
			a.name, a.role, a.status
		 FROM service_account_keys k
		 JOIN service_accounts a ON a.id = k.service_account_id
		 WHERE k.token_hash = ?
		   AND k.status = 'active'
		   AND k.revoked_at IS NULL
		   AND a.status = 'active'
		   AND (k.expires_at IS NULL OR k.expires_at = '' OR k.expires_at > ?)
		 LIMIT 1`,
		tokenHash,
		now.UTC().Format(time.RFC3339),
	)
	var (
		key           APIKey
		expiresAt     sql.NullString
		createdAt     string
		revokedAt     sql.NullString
		lastUsedAt    sql.NullString
		accountName   string
		accountRole   string
		accountStatus string
	)
	if err := row.Scan(
		&key.ID,
		&key.ServiceAccountID,
		&key.Name,
		&key.KeyPrefix,
		&key.TokenHash,
		&key.Status,
		&expiresAt,
		&createdAt,
		&revokedAt,
		&lastUsedAt,
		&accountName,
		&accountRole,
		&accountStatus,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("serviceaccountrepo find key by hash: %w", err)
	}
	key.CreatedAt = parseTime(createdAt)
	key.ExpiresAt = parseNullableTime(expiresAt)
	key.RevokedAt = parseNullableTime(revokedAt)
	key.LastUsedAt = parseNullableTime(lastUsedAt)
	return &KeyWithAccount{
		Key:           key,
		AccountName:   accountName,
		AccountRole:   accountRole,
		AccountStatus: accountStatus,
	}, nil
}

func (r *Repo) TouchKey(ctx context.Context, accountID, keyID string, seenAt time.Time) error {
	value := seenAt.UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE service_account_keys
		    SET last_used_at = ?
		  WHERE id = ? AND service_account_id = ?`,
		value,
		keyID,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("serviceaccountrepo touch key: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE service_accounts
		    SET last_used_at = ?, updated_at = ?
		  WHERE id = ?`,
		value,
		value,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("serviceaccountrepo touch account: %w", err)
	}
	return nil
}

func (r *Repo) RevokeKey(ctx context.Context, accountID, keyID string, revokedAt time.Time) error {
	value := revokedAt.UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		`UPDATE service_account_keys
		    SET status = 'revoked', revoked_at = ?
		  WHERE id = ? AND service_account_id = ?`,
		value,
		keyID,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("serviceaccountrepo revoke key: %w", err)
	}
	return nil
}

func normalizeStatus(status string) string {
	if strings.TrimSpace(status) == "revoked" {
		return "revoked"
	}
	return "active"
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAccount(row scanner) (*ServiceAccount, error) {
	var (
		account    ServiceAccount
		createdAt  string
		updatedAt  string
		lastUsedAt sql.NullString
	)
	if err := row.Scan(
		&account.ID,
		&account.Name,
		&account.Role,
		&account.Status,
		&createdAt,
		&updatedAt,
		&lastUsedAt,
	); err != nil {
		return nil, err
	}
	account.CreatedAt = parseTime(createdAt)
	account.UpdatedAt = parseTime(updatedAt)
	account.LastUsedAt = parseNullableTime(lastUsedAt)
	return &account, nil
}

func scanKey(row scanner) (*APIKey, error) {
	var (
		key       APIKey
		expiresAt sql.NullString
		createdAt string
		revokedAt sql.NullString
		lastUsed  sql.NullString
	)
	if err := row.Scan(
		&key.ID,
		&key.ServiceAccountID,
		&key.Name,
		&key.KeyPrefix,
		&key.TokenHash,
		&key.Status,
		&expiresAt,
		&createdAt,
		&revokedAt,
		&lastUsed,
	); err != nil {
		return nil, err
	}
	key.ExpiresAt = parseNullableTime(expiresAt)
	key.CreatedAt = parseTime(createdAt)
	key.RevokedAt = parseNullableTime(revokedAt)
	key.LastUsedAt = parseNullableTime(lastUsed)
	return &key, nil
}

func parseTime(value string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t
	}
	t, _ := time.Parse(time.RFC3339, value)
	return t
}

func parseNullableTime(value sql.NullString) *time.Time {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	parsed := parseTime(value.String)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}
