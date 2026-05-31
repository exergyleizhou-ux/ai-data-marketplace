package db

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the "pgx5" driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/lei/ai-data-marketplace/backend/migrations"
)

// RunMigrations applies all up-migrations against dsn. It is idempotent:
// already-applied migrations are skipped and ErrNoChange is treated as success.
func RunMigrations(dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, pgxScheme(dsn))
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// pgxScheme rewrites a postgres:// DSN to the pgx5:// scheme that the migrate
// pgx/v5 driver registers under, leaving the rest of the URL intact.
func pgxScheme(dsn string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if len(dsn) >= len(p) && dsn[:len(p)] == p {
			return "pgx5://" + dsn[len(p):]
		}
	}
	return dsn
}
