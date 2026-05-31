package database

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

const (
	tableArtistWatches = "artist_watches"
)

type ArtistWatch struct {
	UserID      string `db:"user_id"`
	ArtistMBzID string `db:"artist_mb_id"`
	Source      string `db:"source"`
	Status      bool   `db:"status"`
}

func (d *DB) AddArtistWatch(ctx context.Context, watch ArtistWatch) error {
	_, err := d.gq.Insert(tableArtistWatches).
		Rows(watch).
		OnConflict(goqu.DoUpdate("artist_mb_id, user_id", goqu.Record{
			"status": goqu.L("excluded.status"),
		})).
		Executor().Exec()

	if err != nil {
		return fmt.Errorf("upsert artist watch: %w", err)
	}

	return nil
}

func (d *DB) ArtistWatch(ctx context.Context, userID string, artistMBzID string) (ArtistWatch, bool, error) {
	res := ArtistWatch{}

	ok, err := d.gq.
		Select("*").
		From(tableArtistWatches).
		Where(
			goqu.C("user_id").Eq(userID),
			goqu.C("artist_mb_id").Eq(artistMBzID),
		).
		Executor().ScanStructContext(ctx, &res)
	if err != nil {
		return res, false, fmt.Errorf("artist watch: %w", err)
	}
	if !ok {
		return ArtistWatch{
			UserID:      userID,
			ArtistMBzID: artistMBzID,
			Status:      false,
		}, false, err
	}

	return res, ok, nil
}
