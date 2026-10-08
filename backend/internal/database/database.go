// Package database owns schema migrations for tables managed by the Go server.
package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// migrationsTable records applied migrations. It is prefixed like every
// Go-owned table so it never collides with the legacy app's tables.
const migrationsTable = "go_schema_migrations"

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies every pending embedded migration. A Postgres advisory lock
// serializes concurrent callers, so several instances can start at once.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (err error) {
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration lock: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close migration connection: %w", closeErr))
		}
	}()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys,
		goose.WithTableName(migrationsTable),
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
