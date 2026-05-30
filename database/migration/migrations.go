package migration

import (
	"context"
	"database/sql"
	"embed"
	_ "embed"
	"io/fs"
	"log/slog"
	"path/filepath"
)

//go:embed sql
var migs embed.FS

type Migrations struct {
	db *sql.DB
}

func New(db *sql.DB) *Migrations {
	return &Migrations{db: db}
}

func (m *Migrations) Exec(ctx context.Context) error {
	entries, err := fs.ReadDir(migs, "sql")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		contents, err := fs.ReadFile(migs, filepath.Join("sql", entry.Name()))
		if err != nil {
			return err
		}

		slog.Info("applying", "migration", entry.Name())
		_, err = m.db.Exec(string(contents))
		if err != nil {
			return err
		}
	}

	return nil
}
