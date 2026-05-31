package migration

import (
	"context"
	"database/sql"
	"embed"
	_ "embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed sql/*.sql
var fs embed.FS

type Migrations struct {
	db *sql.DB
}

func New(db *sql.DB) *Migrations {
	return &Migrations{db: db}
}

func (m *Migrations) Exec(ctx context.Context) error {
	d, err := iofs.New(fs, "sql")
	if err != nil {
		return fmt.Errorf("migrations: source: %w", err)
	}

	dbInst, err := sqlite.WithInstance(m.db, &sqlite.Config{
		MigrationsTable: "migrations",
	})
	if err != nil {
		return fmt.Errorf("migrations: db: %w", err)
	}

	mig, err := migrate.NewWithInstance("iofs", d, "sqlite", dbInst)
	if err != nil {
		return fmt.Errorf("migrations: new: %w", err)
	}

	err = mig.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("migrations: up: %w", err)
	}

	return nil
}
