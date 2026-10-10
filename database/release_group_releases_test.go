package database

import (
	"context"
	"testing"
	"time"

	opt "github.com/shimmerglass/go-optional"
	"github.com/stretchr/testify/require"
)

func TestReleaseGroupReleasesRoundTrip(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "rg1", Name: "rg1"})

		got, err := db.ReleaseGroupReleases(ctx, "rg1")
		require.NoError(t, err)
		require.Empty(t, got)

		now := time.Unix(1000, 0)
		in := []CachedRelease{
			{MBzID: "r2", RecordingIDs: []string{"a", "b"}},
			{MBzID: "r1", RecordingIDs: nil},
		}
		require.NoError(t, db.ReplaceReleaseGroupReleases(ctx, "rg1", in, now))

		got, err = db.ReleaseGroupReleases(ctx, "rg1")
		require.NoError(t, err)
		require.Equal(t, []CachedRelease{
			{MBzID: "r1"},
			{MBzID: "r2", RecordingIDs: []string{"a", "b"}},
		}, got)

		rg, err := db.ReleaseGroup(ctx, "rg1")
		require.NoError(t, err)
		require.NotNil(t, rg.ReleasesFetchedAt)
		require.Equal(t, int64(1000), *rg.ReleasesFetchedAt)

		// replacing with empty clears rows but updates fetched_at
		require.NoError(t, db.ReplaceReleaseGroupReleases(ctx, "rg1", nil, time.Unix(2000, 0)))
		got, err = db.ReleaseGroupReleases(ctx, "rg1")
		require.NoError(t, err)
		require.Empty(t, got)
		rg, err = db.ReleaseGroup(ctx, "rg1")
		require.NoError(t, err)
		require.Equal(t, int64(2000), *rg.ReleasesFetchedAt)
	})
}

func TestReleasesFetchedAtSurvivesPut(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "rg1", Name: "old"})
		require.NoError(t, db.ReplaceReleaseGroupReleases(ctx, "rg1", nil, time.Unix(1000, 0)))

		// like refresh_artist_releases: rewrite metadata from a fresh struct
		require.NoError(t, db.PutReleaseGroup(ctx, "rg1", func(o opt.Option[ReleaseGroup]) ReleaseGroup {
			rg := o.TakeOr(ReleaseGroup{MBzID: "rg1"})
			rg.Name = "new"
			return rg
		}))
		// even a write that carries no fetched_at must not clear it
		require.NoError(t, db.addReleaseGroup(ctx, ReleaseGroup{MBzID: "rg1", Name: "newer"}))

		rg, err := db.ReleaseGroup(ctx, "rg1")
		require.NoError(t, err)
		require.Equal(t, "newer", rg.Name)
		require.NotNil(t, rg.ReleasesFetchedAt)
		require.Equal(t, int64(1000), *rg.ReleasesFetchedAt)
	})
}

func TestDeleteReleaseGroupDeletesCache(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "rg1"})
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "rg2"})
		cr := []CachedRelease{{MBzID: "r1", RecordingIDs: []string{"x"}}}
		require.NoError(t, db.ReplaceReleaseGroupReleases(ctx, "rg1", cr, time.Now()))
		require.NoError(t, db.ReplaceReleaseGroupReleases(ctx, "rg2", cr, time.Now()))

		require.NoError(t, db.DeleteReleaseGroup(ctx, "rg1"))

		got, err := db.ReleaseGroupReleases(ctx, "rg1")
		require.NoError(t, err)
		require.Empty(t, got)
		got, err = db.ReleaseGroupReleases(ctx, "rg2")
		require.NoError(t, err)
		require.Len(t, got, 1)
	})
}
