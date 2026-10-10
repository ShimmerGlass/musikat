package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
)

const tableReleaseGroupReleases = "release_group_releases"

// CachedRelease is a MusicBrainz release of a release group along with the
// IDs of its recordings, as last fetched from MusicBrainz.
type CachedRelease struct {
	MBzID        string
	RecordingIDs []string
}

func (d *DB) ReleaseGroupReleases(ctx context.Context, rgID string) ([]CachedRelease, error) {
	type row struct {
		ReleaseMBzID string `db:"release_mb_id"`
		RecordingIDs string `db:"recording_ids"`
	}

	rows := []row{}
	err := d.gq.
		Select("release_mb_id", "recording_ids").
		From(tableReleaseGroupReleases).
		Where(goqu.C("release_group_mb_id").Eq(rgID)).
		Order(goqu.C("release_mb_id").Asc()).
		Executor().ScanStructsContext(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("release group releases: %w", err)
	}

	res := make([]CachedRelease, 0, len(rows))
	for _, r := range rows {
		var recs []string
		if r.RecordingIDs != "" {
			recs = strings.Split(r.RecordingIDs, ",")
		}
		res = append(res, CachedRelease{MBzID: r.ReleaseMBzID, RecordingIDs: recs})
	}

	return res, nil
}

// ReplaceReleaseGroupReleases replaces the cached releases of a release group
// and records when they were fetched.
func (d *DB) ReplaceReleaseGroupReleases(ctx context.Context, rgID string, releases []CachedRelease, fetchedAt time.Time) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	tx, err := d.gq.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace release group releases: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.
		Delete(tableReleaseGroupReleases).
		Where(goqu.C("release_group_mb_id").Eq(rgID)).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("replace release group releases: %w", err)
	}

	recs := make([]any, 0, len(releases))
	for _, rel := range releases {
		recs = append(recs, goqu.Record{
			"release_group_mb_id": rgID,
			"release_mb_id":       rel.MBzID,
			"recording_ids":       strings.Join(rel.RecordingIDs, ","),
		})
	}

	if len(recs) > 0 {
		_, err = tx.
			Insert(tableReleaseGroupReleases).
			Rows(recs...).
			OnConflict(goqu.DoNothing()).
			Executor().ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("replace release group releases: %w", err)
		}
	}

	_, err = tx.
		Update(tableReleaseGroups).
		Set(goqu.Record{"releases_fetched_at": fetchedAt.Unix()}).
		Where(goqu.C("mb_id").Eq(rgID)).
		Executor().ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("replace release group releases: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("replace release group releases: commit: %w", err)
	}

	return nil
}
