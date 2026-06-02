package database

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/samber/lo"
	"github.com/shimmerglass/musikat/database/migration"
	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
	gq  *goqu.Database

	lock sync.RWMutex
}

func New(cfg Config) (*DB, error) {
	dbURL := url.URL{}
	dbURL.Scheme = "file"
	dbURL.Path = lo.Must(filepath.Abs(cfg.Path))

	query := url.Values{}
	query.Set("_pragma", "journal_mode = WAL")
	dbURL.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", dbURL.String())
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
