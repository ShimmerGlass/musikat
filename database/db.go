package database

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/shimmerglass/musikat/database/migration"
	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
	gq  *goqu.Database
}

func New(cfg Config) (*DB, error) {
	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, err
	}

	mig := migration.New(db)
	err = mig.Exec(context.Background())
	if err != nil {
		return nil, err
	}

	dialect := goqu.Dialect("sqlite3")
	dialect.DB(db)

	return &DB{
		sql: db,
		gq:  dialect.DB(db),
	}, nil
}
