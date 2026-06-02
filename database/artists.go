package database

import (
	"context"
	"fmt"
	"path"

	"github.com/doug-martin/goqu/v9"
	"github.com/samber/lo"
)

const (
	tableArtists = "artists"
)

var ErrArtistNotFound = fmt.Errorf("artist not found")

type Artist struct {
	MBzID       string `db:"mb_id"`
	Name        string `db:"name"`
	RefreshedAt *int64 `db:"refreshed_at"`
}

func (a Artist) MBzURL() string {
	return fmt.Sprintf("https://musicbrainz.org/artist/%s", a.MBzID)
}

func (a Artist) UIPath() string {
	return path.Join("/artists", a.MBzID)
}

type ArtistWithStats struct {
	Artist

	Present int
	Missing int
}

func (d *DB) AddArtist(ctx context.Context, artist Artist) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	_, err := d.gq.
		Insert(tableArtists).
		Rows(artist).
		OnConflict(goqu.DoUpdate("mb_id", goqu.Record{
			"name":         goqu.L("excluded.name"),
			"refreshed_at": goqu.L("excluded.refreshed_at"),
		})).
		Executor().ExecContext(ctx)

	if err != nil {
		return fmt.Errorf("upsert artist: %w", err)
	}

	return nil
}

func (d *DB) Artist(ctx context.Context, mbzID string) (Artist, error) {
	artist := Artist{}
	ok, err := d.gq.
		Select("*").
		From(tableArtists).
		Where(goqu.Ex{
			"mb_id": mbzID,
		}).
		Executor().ScanStructContext(ctx, &artist)
	if err != nil {
		return Artist{}, fmt.Errorf("list watched artists: select: %w", err)
	}
	if !ok {
		return Artist{}, ErrArtistNotFound
	}

	return artist, nil
}

func (d *DB) Artists(ctx context.Context) ([]Artist, error) {
	res := []Artist{}

	err := d.gq.
		Select("*").
		From(tableArtists).
		Executor().ScanStructsContext(ctx, &res)
	if err != nil {
		return nil, fmt.Errorf("list artists: %w", err)
	}

	return res, nil
}

func (d *DB) WatchedArtists(ctx context.Context) ([]Artist, error) {
	scanner, err := d.gq.
		Select(fmt.Sprintf("%s.*", tableArtists)).
		From(tableArtists).
		Join(goqu.T(tableArtistWatches), goqu.On(goqu.I("artist_mb_id").Eq(goqu.I("mb_id")))).
		Where(goqu.Ex{
			"status": 1,
		}).
		Executor().ScannerContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list watched artists: select: %w", err)
	}
	defer func() { _ = scanner.Close() }()

	seen := map[string]bool{}
	res := []Artist{}

	for scanner.Next() {
		a := Artist{}

		err = scanner.ScanStruct(&a)
		if err != nil {
			return nil, fmt.Errorf("list watched artists: scan: %w", err)
		}

		if seen[a.MBzID] {
			continue
		}
		seen[a.MBzID] = true

		res = append(res, a)
	}

	return res, nil
}

func (d *DB) UserWatchedArtists(ctx context.Context, user User) ([]Artist, error) {
	scanner, err := d.gq.
		Select(fmt.Sprintf("%s.*", tableArtists)).
		From(tableArtists).
		Join(goqu.T(tableArtistWatches), goqu.On(goqu.I("artist_mb_id").Eq(goqu.I("mb_id")))).
		Where(goqu.Ex{
			"user_id": user.ID,
			"status":  1,
		}).
		Executor().ScannerContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list watched artists: select: %w", err)
	}
	defer func() { _ = scanner.Close() }()

	seen := map[string]bool{}
	res := []Artist{}

	for scanner.Next() {
		a := Artist{}

		err = scanner.ScanStruct(&a)
		if err != nil {
			return nil, fmt.Errorf("list watched artists: scan: %w", err)
		}

		if seen[a.MBzID] {
			continue
		}
		seen[a.MBzID] = true

		res = append(res, a)
	}

	return res, nil
}

func (d *DB) UserWatchedArtistsWithStats(ctx context.Context, user User) ([]ArtistWithStats, error) {
	artists, err := d.UserWatchedArtists(ctx, user)
	if err != nil {
		return nil, err
	}

	res := []ArtistWithStats{}
	for _, artist := range artists {
		rgs, err := d.ArtistReleaseGroups(ctx, artist)
		if err != nil {
			return nil, err
		}

		present := lo.CountBy(rgs, func(rg ReleaseGroup) bool { return rg.LibraryStatus == LibraryStatusPresent })
		missing := lo.CountBy(rgs, func(rg ReleaseGroup) bool { return rg.LibraryStatus == LibraryStatusMissing })

		res = append(res, ArtistWithStats{
			Artist:  artist,
			Present: present,
			Missing: missing,
		})
	}

	return res, nil
}
