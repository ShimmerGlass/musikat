package database

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9/exp"
)

const (
	tableArtistWatches = "artist_watches"
)

type ArtistWatch struct {
	UserID      string `db:"user_id"`
	ArtistMBzID string `db:"artist_mb_id"`
	Source      string `db:"source"`
}

func (d *DB) AddArtistWatch(ctx context.Context, watch ArtistWatch) error {
	_, err := d.gq.Insert(tableArtistWatches).
		Rows(watch).
		OnConflict(exp.NewDoNothingConflictExpression()).Executor().Exec()

	if err != nil {
		return fmt.Errorf("upsert artist watch: %w", err)
	}

	return nil
}
