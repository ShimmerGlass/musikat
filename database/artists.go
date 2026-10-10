package database

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/samber/lo"
	opt "github.com/shimmerglass/go-optional"
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

	Upcoming int
	Present  int
	Missing  int
}

func (d *DB) addArtist(ctx context.Context, artist Artist) error {
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

func (d *DB) PutArtist(ctx context.Context, id string, do func(opt.Option[Artist]) Artist) error {
	var newArtist Artist
	existing, err := d.Artist(ctx, id)
	if errors.Is(err, ErrArtistNotFound) {
		newArtist = do(opt.None[Artist]())
	} else if err != nil {
		return err
	} else {
		newArtist = do(opt.Some(existing))
	}

	return d.addArtist(ctx, newArtist)
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
		return Artist{}, fmt.Errorf("artist by id: %w", err)
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
		Order(goqu.I("name").Asc()).
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
	now := time.Now()

	res := []ArtistWithStats{}
	if len(artists) == 0 {
		return res, nil
	}

	watchList, err := d.UserArtistWatches(ctx, user)
	if err != nil {
		return nil, err
	}
	watches := make(map[string]ArtistWatch, len(watchList))
	for _, w := range watchList {
		watches[w.ArtistMBzID] = w
	}

	ids := make([]any, 0, len(artists))
	for _, a := range artists {
		ids = append(ids, a.MBzID)
	}

	type row struct {
		ReleaseGroup
		ArtistMBzID string `db:"artist_mb_id"`
	}

	rows := []row{}
	err = d.gq.
		Select(
			goqu.L("release_groups.*"),
			goqu.T(tableReleaseGroupArtists).Col("artist_mb_id"),
		).
		From(tableReleaseGroups).
		Join(goqu.T(tableReleaseGroupArtists), goqu.On(goqu.I("release_group_mb_id").Eq(goqu.I("mb_id")))).
		Where(goqu.C("artist_mb_id").In(ids...)).
		Order(goqu.I("release_date").Desc()).
		Executor().ScanStructsContext(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("list watched artists release groups: %w", err)
	}

	byArtist := make(map[string][]ReleaseGroup, len(artists))
	for _, r := range rows {
		byArtist[r.ArtistMBzID] = append(byArtist[r.ArtistMBzID], r.ReleaseGroup)
	}

	for _, artist := range artists {
		// A missing watch yields a zero-value watch, as ArtistWatch does.
		watch, ok := watches[artist.MBzID]
		if !ok {
			watch = ArtistWatch{UserID: user.ID, ArtistMBzID: artist.MBzID}
		}

		rgs := lo.Filter(byArtist[artist.MBzID], func(rg ReleaseGroup, _ int) bool {
			return watch.Watches(rg)
		})

		present := lo.CountBy(rgs, func(rg ReleaseGroup) bool { return rg.LibraryStatus == LibraryStatusPresent })
		missing := lo.CountBy(rgs, func(rg ReleaseGroup) bool {
			return rg.LibraryStatus == LibraryStatusMissing && rg.ReleaseTime().Before(now)
		})
		upcoming := lo.CountBy(rgs, func(rg ReleaseGroup) bool {
			return rg.ReleaseTime().After(now)
		})

		res = append(res, ArtistWithStats{
			Artist: artist,

			Upcoming: upcoming,
			Present:  present,
			Missing:  missing,
		})
	}

	return res, nil
}
