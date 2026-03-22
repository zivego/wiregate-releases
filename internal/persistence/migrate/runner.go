package migrate

import (
	"context"
	"fmt"

	"github.com/zivego/wiregate/internal/persistence/migrations"
)

// Executor abstracts SQL execution so the migration planner remains DB-driver agnostic.
type Executor interface {
	EnsureSchemaMigrations(ctx context.Context) error
	ExecContext(ctx context.Context, query string) error
	HasMigration(ctx context.Context, name string) (bool, error)
	RecordMigration(ctx context.Context, name string) error
}

func ApplyAll(ctx context.Context, exec Executor) error {
	if err := exec.EnsureSchemaMigrations(ctx); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	all, err := migrations.LoadAll()
	if err != nil {
		return err
	}

	for _, migration := range all {
		applied, err := exec.HasMigration(ctx, migration.Name)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", migration.Name, err)
		}
		if applied {
			continue
		}
		if execErr := exec.ExecContext(ctx, migration.SQL); execErr != nil {
			return fmt.Errorf("apply migration %s: %w", migration.Name, execErr)
		}
		if execErr := exec.RecordMigration(ctx, migration.Name); execErr != nil {
			return fmt.Errorf("record migration %s: %w", migration.Name, execErr)
		}
	}
	return nil
}
