package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/samber/lo"
)

const (
	tableArtistWatches = "artist_watches"
)

type ArtistWatch struct {
	UserID           string `db:"user_id"`
	ArtistMBzID      string `db:"artist_mb_id"`
	Source           string `db:"source"`
	Status           bool   `db:"status"`
	AddedAt          int64  `db:"added_at"`
	XXPrimaryTypes   string `db:"primary_types"`
	XXSecondaryTypes string `db:"secondary_types"`
}

func (w ArtistWatch) PrimaryTypes() []string {
	return strings.Split(w.XXPrimaryTypes, ",")
}

func (w ArtistWatch) SecondaryTypes() []string {
	return strings.Split(w.XXSecondaryTypes, ",")
}

func (w ArtistWatch) Watches(rg ReleaseGroup) bool {
	secTypes := w.SecondaryTypes()
	rgSecTypes := rg.SecondaryTypes()
	return lo.Contains(w.PrimaryTypes(), lo.CoalesceOrEmpty(rg.PrimaryType, "Unknown")) && (len(rgSecTypes) == 0 || len(lo.Intersect(secTypes, rg.SecondaryTypes())) > 0)
}

func (d *DB) AddArtistWatch(ctx context.Context, watch ArtistWatch) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	_, err := d.gq.Insert(tableArtistWatches).
		Rows(watch).
		OnConflict(goqu.DoUpdate("artist_mb_id, user_id", goqu.Record{
			"status":          goqu.L("excluded.status"),
			"primary_types":   goqu.L("excluded.primary_types"),
			"secondary_types": goqu.L("excluded.secondary_types"),
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

func (d *DB) UserArtistWatches(ctx context.Context, user User) ([]ArtistWatch, error) {
	res := []ArtistWatch{}

	err := d.gq.
		Select("*").
		From(tableArtistWatches).
		Where(
			goqu.C("user_id").Eq(user.ID),
		).
		Executor().ScanStructsContext(ctx, &res)
	if err != nil {
		return nil, fmt.Errorf("user artist watched: %w", err)
	}

	return res, nil
}
