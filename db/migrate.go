package db

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	// postgres driver for golang-migrate
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// lib/pq is the underlying driver the postgres migrate adapter uses
	_ "github.com/lib/pq"
)

// migrations embeds the SQL files at compile time so the binary is
// self-contained — no need to ship the sql files alongside the binary.
//
//go:embed migrations/*.sql
var migrations embed.FS

// RunMigrations applies any pending "up" migrations against the given
// Postgres URL and returns nil if the schema is already up to date.
func RunMigrations(dbURL string) (err error) {
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("db: open migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dbURL)
	if err != nil {
		return fmt.Errorf("db: create migrator: %w", err)
	}
	defer func() {
		errSource, errDB := m.Close()
		err = errors.Join(err, errSource, errDB)
	}()

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("db: run migrations: %w", err)
	}

	return nil
}
