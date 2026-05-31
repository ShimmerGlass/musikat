package database

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

type Stats struct {
	Artists       int
	ReleaseGroups int
	Watches       int
}

func (d *DB) Stats(ctx context.Context) (stats Stats, err error) {
	_, err = d.gq.
		Select(goqu.L("count(*)")).
		From(tableArtists).
		Executor().ScanValContext(ctx, &stats.Artists)
	if err != nil {
		err = fmt.Errorf("stats: artists: %w", err)
		return
	}

	_, err = d.gq.
		Select(goqu.L("count(*)")).
		From(tableReleaseGroups).
		Executor().ScanValContext(ctx, &stats.ReleaseGroups)
	if err != nil {
		err = fmt.Errorf("stats: release groups: %w", err)
		return
	}

	_, err = d.gq.
		Select(goqu.L("count(*)")).
		From(tableArtistWatches).
		Executor().ScanValContext(ctx, &stats.Watches)
	if err != nil {
		err = fmt.Errorf("stats: watches: %w", err)
		return
	}

	return
}
