package dnsconfigrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

const singletonID = "default"

type Config struct {
	ID            string
	Enabled       bool
	Servers       []string
	SearchDomains []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Get(ctx context.Context) (*Config, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, enabled, servers_json, search_domains_json, created_at, updated_at
		   FROM dns_configs
		  WHERE id = ?
		  LIMIT 1`,
		singletonID,
	)
	config, err := scanConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dnsconfigrepo get: %w", err)
	}
	return config, nil
}

func (r *Repo) Upsert(ctx context.Context, config Config) error {
	serversJSON, err := json.Marshal(config.Servers)
	if err != nil {
		return fmt.Errorf("dnsconfigrepo marshal servers: %w", err)
	}
	searchDomainsJSON, err := json.Marshal(config.SearchDomains)
	if err != nil {
		return fmt.Errorf("dnsconfigrepo marshal search domains: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO dns_configs (id, enabled, servers_json, search_domains_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   enabled = excluded.enabled,
		   servers_json = excluded.servers_json,
		   search_domains_json = excluded.search_domains_json,
		   updated_at = excluded.updated_at`,
		singletonID,
		config.Enabled,
		string(serversJSON),
		string(searchDomainsJSON),
		config.CreatedAt.UTC().Format(time.RFC3339Nano),
		config.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("dnsconfigrepo upsert: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanConfig(row scanner) (*Config, error) {
	var config Config
	var enabled bool
	var serversJSON string
	var searchDomainsJSON string
	var createdAt string
	var updatedAt string
	if err := row.Scan(&config.ID, &enabled, &serversJSON, &searchDomainsJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	config.Enabled = enabled
	if err := json.Unmarshal([]byte(serversJSON), &config.Servers); err != nil {
		return nil, fmt.Errorf("dnsconfigrepo decode servers: %w", err)
	}
	if err := json.Unmarshal([]byte(searchDomainsJSON), &config.SearchDomains); err != nil {
		return nil, fmt.Errorf("dnsconfigrepo decode search domains: %w", err)
	}
	config.CreatedAt = parseTime(createdAt)
	config.UpdatedAt = parseTime(updatedAt)
	return &config, nil
}

func parseTime(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}
