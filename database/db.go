package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/doug-martin/goqu/v9/exec"
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

func scan[T any](scanner exec.Scanner) ([]T, error) {
	res := []T{}

	for scanner.Next() {
		var el T

		err := scanner.ScanStruct(&el)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		res = append(res, el)
	}

	return res, nil
}
