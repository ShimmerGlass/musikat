package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStats(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1", "u2")
		got, err := db.Stats(ctx)
		require.NoError(t, err)
		require.Equal(t, Stats{}, got)

		a1 := seedArtist(t, ctx, db, "a1")
		a2 := seedArtist(t, ctx, db, "a2")
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", ReleaseDate: "2020"}, a1, a2)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r2", ReleaseDate: "2021"}, a1)
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true}))
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u2", ArtistMBzID: "a1", Status: false}))
		// upsert does not add a row
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: false}))

		got, err = db.Stats(ctx)
		require.NoError(t, err)
		require.Equal(t, Stats{Artists: 2, ReleaseGroups: 2, Watches: 2}, got)

		require.NoError(t, db.RemoveReleaseGroupArtist(ctx, "r2", "a1"))
		got, err = db.Stats(ctx)
		require.NoError(t, err)
		require.Equal(t, Stats{Artists: 2, ReleaseGroups: 1, Watches: 2}, got)
	})
}
