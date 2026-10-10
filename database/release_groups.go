package database

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	opt "github.com/shimmerglass/go-optional"
)

const (
	tableReleaseGroups       = "release_groups"
	tableReleaseGroupArtists = "release_group_artists"
)

var ErrReleaseGroupNotFound = fmt.Errorf("release group not found")

type LibraryStatus int

const (
	LibraryStatusUnknown      LibraryStatus = 0
	LibraryStatusPresent      LibraryStatus = 1
	LibraryStatusMissing      LibraryStatus = 2
	LibraryStatusSongsPresent LibraryStatus = 3
)

type ReleaseGroup struct {
	MBzID                 string        `db:"mb_id"`
	Name                  string        `db:"name"`
	PrimaryType           string        `db:"primary_type"`
	XXSecondaryTypes      string        `db:"secondary_type"`
	ReleaseDate           string        `db:"release_date"`
	LibraryStatus         LibraryStatus `db:"in_library"`
	InLibraryReleaseMBzID string        `db:"in_library_release_mb_id"`
	ReleasesFetchedAt     *int64        `db:"releases_fetched_at"`

	Artists []Artist `db:"-" yaml:"artists"`
}

func (r ReleaseGroup) SecondaryTypes() []string {
	if r.XXSecondaryTypes == "" {
		return nil
	}
	return strings.Split(r.XXSecondaryTypes, ",")
}

func (r ReleaseGroup) Year() string {
	year, _, _ := strings.Cut(r.ReleaseDate, "-")
	return year
}

func (r ReleaseGroup) MBzURL() string {
	return fmt.Sprintf("https://musicbrainz.org/release-group/%s", r.MBzID)
}

func (r ReleaseGroup) ReleaseTime() time.Time {
	year, month, day, _ := r.parseReleaseDate()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// ReleaseEnd returns the exclusive end of the release period: the start of the
// day after a full date, of the month after a year-month date, or of the year
// after a year-only date. Empty or unparsable dates follow ReleaseTime (year 0
// is treated as a year-only date).
func (r ReleaseGroup) ReleaseEnd() time.Time {
	year, month, day, precision := r.parseReleaseDate()
	switch precision {
	case precisionDay:
		return time.Date(year, month, day+1, 0, 0, 0, 0, time.UTC)
	case precisionMonth:
		return time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
}

// IsFullDate reports whether the release date has day precision.
func (r ReleaseGroup) IsFullDate() bool {
	_, _, _, precision := r.parseReleaseDate()
	return precision == precisionDay
}

const (
	precisionYear = iota
	precisionMonth
	precisionDay
)

// parseReleaseDate parses year, month and day, defaulting unparsable parts to
// 0 (year) or 1 (month, day). precision is the number of leading parts that
// parsed successfully, e.g. "2005-??-31" has year precision.
func (r ReleaseGroup) parseReleaseDate() (year int, month time.Month, day int, precision int) {
	month = time.January
	day = 1

	parts := strings.Split(r.ReleaseDate, "-")

	year, _ = strconv.Atoi(parts[0])
	if len(parts) > 1 {
		m, err := strconv.Atoi(parts[1])
		if err == nil {
			month = time.Month(m)
			precision = precisionMonth
		}
	}
	if len(parts) > 2 {
		d, err := strconv.Atoi(parts[2])
		if err == nil {
			day = d
			if precision == precisionMonth {
				precision = precisionDay
			}
		}
	}

	return year, month, day, precision
}

func (d *DB) addReleaseGroup(ctx context.Context, rg ReleaseGroup) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	// releases_fetched_at is deliberately absent from the conflict update: it
	// is owned by ReplaceReleaseGroupReleases and must survive upserts.
	_, err := d.gq.Insert(tableReleaseGroups).
		Rows(rg).
		OnConflict(goqu.DoUpdate("mb_id", goqu.Record{
			"name":                     goqu.L("excluded.name"),
			"in_library":               goqu.L("excluded.in_library"),
			"in_library_release_mb_id": goqu.L("excluded.in_library_release_mb_id"),
			"primary_type":             goqu.L("excluded.primary_type"),
			"secondary_type":           goqu.L("excluded.secondary_type"),
			"release_date":             goqu.L("excluded.release_date"),
		})).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("add release group: %w", err)
	}

	return nil
}

func (d *DB) PutReleaseGroup(ctx context.Context, id string, do func(opt.Option[ReleaseGroup]) ReleaseGroup) error {
	var newReleaseGroup ReleaseGroup
	existing, err := d.ReleaseGroup(ctx, id)
	if errors.Is(err, ErrReleaseGroupNotFound) {
		newReleaseGroup = do(opt.None[ReleaseGroup]())
	} else if err != nil {
		return err
	} else {
		newReleaseGroup = do(opt.Some(existing))
	}

	return d.addReleaseGroup(ctx, newReleaseGroup)
}

func (d *DB) ReleaseGroup(ctx context.Context, mbzID string) (ReleaseGroup, error) {
	rg := ReleaseGroup{}
	ok, err := d.gq.
		Select("*").
		From(tableReleaseGroups).
		Where(goqu.Ex{
			"mb_id": mbzID,
		}).
		Executor().ScanStructContext(ctx, &rg)
	if err != nil {
		return ReleaseGroup{}, fmt.Errorf("release group by id: %w", err)
	}
	if !ok {
		return ReleaseGroup{}, ErrReleaseGroupNotFound
	}

	return rg, nil
}

// RemoveReleaseGroupArtist unlinks an artist from a release group. If no
// artist remains linked, the release group itself is deleted along with its
// notifications and cached releases.
func (d *DB) RemoveReleaseGroupArtist(ctx context.Context, rgID, artistID string) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	tx, err := d.gq.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("remove release group artist: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.
		Delete(tableReleaseGroupArtists).
		Where(goqu.Ex{
			"release_group_mb_id": rgID,
			"artist_mb_id":        artistID,
		}).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("remove release group artist: %w", err)
	}

	var remaining int
	_, err = tx.
		Select(goqu.COUNT("*")).
		From(tableReleaseGroupArtists).
		Where(goqu.C("release_group_mb_id").Eq(rgID)).
		Executor().ScanValContext(ctx, &remaining)
	if err != nil {
		return fmt.Errorf("remove release group artist: count remaining: %w", err)
	}

	if remaining == 0 {
		err = deleteReleaseGroupTx(ctx, tx, rgID)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("remove release group artist: commit: %w", err)
	}

	return nil
}

func deleteReleaseGroupTx(ctx context.Context, tx *goqu.TxDatabase, mbzID string) error {
	_, err := tx.
		Delete(tableReleaseGroupArtists).
		Where(goqu.Ex{
			"release_group_mb_id": mbzID,
		}).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete release group artists: %w", err)
	}

	_, err = tx.
		Delete(tableReleaseGroupNotifications).
		Where(
			goqu.C("release_group_mb_id").Eq(mbzID),
		).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete release group notifications: %w", err)
	}

	_, err = tx.
		Delete(tableReleaseGroupReleases).
		Where(goqu.C("release_group_mb_id").Eq(mbzID)).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete release group releases: %w", err)
	}

	_, err = tx.
		Delete(tableReleaseGroups).
		Where(goqu.Ex{
			"mb_id": mbzID,
		}).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete release group: %w", err)
	}

	return nil
}

func (d *DB) ReplaceReleaseGroupArtists(ctx context.Context, rg ReleaseGroup) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	tx, err := d.gq.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace release group artists: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.
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

	if len(recs) > 0 {
		_, err = tx.
			Insert(tableReleaseGroupArtists).
			Rows(recs...).
			OnConflict(goqu.DoNothing()).
			Executor().ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("replace release group artists: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("replace release group artists: commit: %w", err)
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

// ArtistReleaseGroups returns the release groups of an artist, newest first,
// with their Artists populated.
func (d *DB) ArtistReleaseGroups(ctx context.Context, artist Artist) ([]ReleaseGroup, error) {
	rgs, err := d.artistReleaseGroups(ctx, artist)
	if err != nil {
		return nil, err
	}

	if len(rgs) == 0 {
		return rgs, nil
	}

	ids := make([]any, 0, len(rgs))
	for _, rg := range rgs {
		ids = append(ids, rg.MBzID)
	}

	type row struct {
		Artist
		ReleaseGroupMBzID string `db:"release_group_mb_id"`
	}

	rows := []row{}
	err = d.gq.
		Select(
			goqu.T(tableArtists).Col("mb_id"),
			goqu.T(tableArtists).Col("name"),
			goqu.T(tableArtists).Col("refreshed_at"),
			goqu.T(tableReleaseGroupArtists).Col("release_group_mb_id"),
		).
		From(tableArtists).
		Join(goqu.T(tableReleaseGroupArtists), goqu.On(goqu.I("artist_mb_id").Eq(goqu.I("mb_id")))).
		Where(goqu.C("release_group_mb_id").In(ids...)).
		Executor().ScanStructsContext(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("list release groups artists: %w", err)
	}

	byRG := make(map[string][]Artist, len(rgs))
	for _, r := range rows {
		byRG[r.ReleaseGroupMBzID] = append(byRG[r.ReleaseGroupMBzID], r.Artist)
	}

	for i := range rgs {
		artists := byRG[rgs[i].MBzID]
		if artists == nil {
			artists = []Artist{}
		}
		rgs[i].Artists = artists
	}

	return rgs, nil
}

// ArtistReleaseGroupsWithoutArtists is like ArtistReleaseGroups but leaves
// ReleaseGroup.Artists unset, saving a query.
func (d *DB) ArtistReleaseGroupsWithoutArtists(ctx context.Context, artist Artist) ([]ReleaseGroup, error) {
	return d.artistReleaseGroups(ctx, artist)
}
