package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
)

const (
	tableReleaseGroups       = "release_groups"
	tableReleaseGroupArtists = "release_group_artists"
)

type LibraryStatus int

const (
	LibraryStatusUnknown LibraryStatus = 0
	LibraryStatusPresent LibraryStatus = 1
	LibraryStatusMissing LibraryStatus = 2
)

type ReleaseGroup struct {
	MBzID                 string        `db:"mb_id"`
	Name                  string        `db:"name"`
	PrimaryType           string        `db:"primary_type"`
	SecondaryType         string        `db:"secondary_type"`
	ReleaseDate           string        `db:"release_date"`
	LibraryStatus         LibraryStatus `db:"in_library"`
	InLibraryReleaseMBzID string        `db:"in_library_release_mb_id"`

	Artists []Artist `db:"-" yaml:"artists"`
}

func (r ReleaseGroup) MBzURL() string {
	return fmt.Sprintf("https://musicbrainz.org/release-group/%s", r.MBzID)
}

func (r ReleaseGroup) ReleaseTime() time.Time {
	year := 0
	month := time.January
	day := 1

	parts := strings.Split(r.ReleaseDate, "-")

	if len(parts) > 0 {
		year, _ = strconv.Atoi(parts[0])
	}
	if len(parts) > 1 {
		m, err := strconv.Atoi(parts[1])
		if err == nil {
			month = time.Month(m)
		}
	}
	if len(parts) > 2 {
		d, err := strconv.Atoi(parts[2])
		if err == nil {
			day = d
		}
	}

	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func (d *DB) AddReleaseGroup(ctx context.Context, rg ReleaseGroup) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	_, err := d.gq.Insert(tableReleaseGroups).
		Rows(rg).
		OnConflict(goqu.DoUpdate("mb_id", goqu.Record{
			"name":                     goqu.L("excluded.name"),
			"in_library":               goqu.L("excluded.in_library"),
			"in_library_release_mb_id": goqu.L("excluded.in_library_release_mb_id"),
			"primary_type":             goqu.L("excluded.primary_type"),
			"secondary_type":           goqu.L("excluded.secondary_type"),
		})).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("add release group: %w", err)
	}

	return nil
}

func (d *DB) AddReleaseGroupWithArtists(ctx context.Context, rg ReleaseGroup) error {
	err := d.AddReleaseGroup(ctx, rg)
	if err != nil {
		return err
	}

	err = d.replaceReleaseGroupArtists(ctx, rg)
	if err != nil {
		return fmt.Errorf("add release group: %w", err)
	}

	return nil
}

func (d *DB) replaceReleaseGroupArtists(ctx context.Context, rg ReleaseGroup) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	_, err := d.gq.
		Delete(tableReleaseGroupArtists).
		Where(goqu.Ex{
			"release_group_mb_id": rg.MBzID,
		}).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("replace release group artists: %w", err)
	}

	recs := make([]any, 0, len(rg.Artists))
	for _, artist := range rg.Artists {
		recs = append(recs, goqu.Record{
			"release_group_mb_id": rg.MBzID,
			"artist_mb_id":        artist.MBzID,
		})
	}

	_, err = d.gq.
		Insert(tableReleaseGroupArtists).
		Rows(recs...).
		OnConflict(goqu.DoNothing()).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("replace release group artists: %w", err)
	}

	return nil
}

func (d *DB) ReleaseGroups(ctx context.Context) ([]ReleaseGroup, error) {
	res := []ReleaseGroup{}

	err := d.gq.
		Select("*").
		From(tableReleaseGroups).
		Executor().ScanStructsContext(ctx, &res)
	if err != nil {
		return nil, fmt.Errorf("list release groups: %w", err)
	}

	return res, nil
}

func (d *DB) artistReleaseGroups(ctx context.Context, artist Artist) ([]ReleaseGroup, error) {
	res := []ReleaseGroup{}

	err := d.gq.
		Select(fmt.Sprintf("%s.*", tableReleaseGroups)).
		From(tableReleaseGroups).
		Join(goqu.T(tableReleaseGroupArtists), goqu.On(goqu.I("release_group_mb_id").Eq(goqu.I("mb_id")))).
		Where(goqu.C("artist_mb_id").Eq(artist.MBzID)).
		Order(goqu.I("release_date").Desc()).
		Executor().ScanStructsContext(ctx, &res)
	if err != nil {
		return nil, fmt.Errorf("list artist release groups: %w", err)
	}

	return res, nil
}

func (d *DB) ArtistReleaseGroups(ctx context.Context, artist Artist) ([]ReleaseGroup, error) {
	rgs, err := d.artistReleaseGroups(ctx, artist)
	if err != nil {
		return nil, err
	}

	for i, rg := range rgs {
		artists, err := d.releaseGroupArtists(ctx, rg)
		if err != nil {
			return nil, err
		}

		rg.Artists = artists
		rgs[i] = rg
	}

	return rgs, nil
}

func (d *DB) releaseGroupArtists(ctx context.Context, rg ReleaseGroup) ([]Artist, error) {
	artists := []Artist{}
	err := d.gq.
		Select(fmt.Sprintf("%s.*", tableArtists)).
		From(tableArtists).
		Join(goqu.T(tableReleaseGroupArtists), goqu.On(goqu.I("artist_mb_id").Eq(goqu.I("mb_id")))).
		Where(goqu.C("release_group_mb_id").Eq(rg.MBzID)).
		Executor().ScanStructsContext(ctx, &artists)
	if err != nil {
		return nil, fmt.Errorf("list release groups artists: %w", err)
	}

	return artists, nil
}
