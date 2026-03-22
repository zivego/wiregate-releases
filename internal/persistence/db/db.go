package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Engine string

const (
	EngineSQLite   Engine = "sqlite"
	EnginePostgres Engine = "postgres"
)

// Handle wraps sql.DB with dialect-aware query rebinding.
type Handle struct {
	*sql.DB
	engine Engine
}

// Open preserves the existing SQLite-first behavior used by tests and local dev.
func Open(path string) (*Handle, error) {
	return OpenSQLite(path)
}

func OpenSQLite(path string) (*Handle, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	handle := &Handle{DB: sqlDB, engine: EngineSQLite}
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		if _, err := handle.DB.Exec(p); err != nil {
			_ = handle.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	if err := handle.PingContext(context.Background()); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return handle, nil
}

func OpenPostgres(dsn string) (*Handle, error) {
	sqlDB, err := sql.Open("pgx", strings.TrimSpace(dsn))
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	handle := &Handle{DB: sqlDB, engine: EnginePostgres}
	if err := handle.PingContext(context.Background()); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return handle, nil
}

func (h *Handle) Engine() Engine {
	if h == nil {
		return EngineSQLite
	}
	return h.engine
}

func (h *Handle) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return h.DB.ExecContext(ctx, h.Rebind(query), args...)
}

func (h *Handle) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return h.DB.QueryContext(ctx, h.Rebind(query), args...)
}

func (h *Handle) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return h.DB.QueryRowContext(ctx, h.Rebind(query), args...)
}

func (h *Handle) Rebind(query string) string {
	if h == nil || h.engine != EnginePostgres || !strings.Contains(query, "?") {
		return query
	}

	var builder strings.Builder
	builder.Grow(len(query) + 8)
	argIndex := 1
	for len(query) > 0 {
		r, size := utf8.DecodeRuneInString(query)
		query = query[size:]
		if r == '?' {
			builder.WriteString(fmt.Sprintf("$%d", argIndex))
			argIndex++
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// Executor wraps Handle to satisfy migrate.Executor.
type Executor struct {
	db *Handle
}

func NewExecutor(db *Handle) *Executor {
	return &Executor{db: db}
}

func (e *Executor) EnsureSchemaMigrations(ctx context.Context) error {
	if e == nil || e.db == nil {
		return fmt.Errorf("migration executor is not configured")
	}
	createTable := `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`
	_, err := e.db.ExecContext(ctx, createTable)
	return err
}

func (e *Executor) ExecContext(ctx context.Context, query string) error {
	_, err := e.db.ExecContext(ctx, query)
	return err
}

func (e *Executor) HasMigration(ctx context.Context, name string) (bool, error) {
	var count int
	err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (e *Executor) RecordMigration(ctx context.Context, name string) error {
	_, err := e.db.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?)`, name)
	return err
}
