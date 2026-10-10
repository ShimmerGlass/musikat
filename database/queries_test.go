package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func seedArtist(t *testing.T, ctx context.Context, db *DB, id string) Artist {
	a := Artist{MBzID: id, Name: id}
	require.NoError(t, db.addArtist(ctx, a))
	return a
}

func seedRG(t *testing.T, ctx context.Context, db *DB, rg ReleaseGroup, artists ...Artist) ReleaseGroup {
	require.NoError(t, db.addReleaseGroup(ctx, rg))
	rg.Artists = artists
	require.NoError(t, db.ReplaceReleaseGroupArtists(ctx, rg))
	return rg
}

func ids(as []Artist) []string {
	r := []string{}
	for _, a := range as {
		r = append(r, a.MBzID)
	}
	return r
}

func TestArtistReleaseGroups(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		a1 := seedArtist(t, ctx, db, "a1")
		a2 := seedArtist(t, ctx, db, "a2")
		a3 := seedArtist(t, ctx, db, "a3")

		rgs, err := db.ArtistReleaseGroups(ctx, a1)
		require.NoError(t, err)
		require.Empty(t, rgs)

		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", ReleaseDate: "2020-01-01"}, a1, a2)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r2", ReleaseDate: "2022-01-01"}, a1)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r3", ReleaseDate: "2021-01-01"}, a3)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r4", ReleaseDate: "2019-01-01"})

		rgs, err = db.ArtistReleaseGroups(ctx, a1)
		require.NoError(t, err)
		require.Len(t, rgs, 2)
		require.Equal(t, "r2", rgs[0].MBzID)
		require.Equal(t, []string{"a1"}, ids(rgs[0].Artists))
		require.Equal(t, "r1", rgs[1].MBzID)
		require.ElementsMatch(t, []string{"a1", "a2"}, ids(rgs[1].Artists))

		rgs, err = db.ArtistReleaseGroupsWithoutArtists(ctx, a3)
		require.NoError(t, err)
		require.Len(t, rgs, 1)
		require.Nil(t, rgs[0].Artists)
	})
}

func TestUserWatchedArtistsWithStats(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		user := User{ID: "u1", Name: "u1"}
		require.NoError(t, db.CreateUser(ctx, user))

		a1 := seedArtist(t, ctx, db, "a1")
		a2 := seedArtist(t, ctx, db, "a2")
		for _, a := range []Artist{a1, a2} {
			require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{
				UserID: user.ID, ArtistMBzID: a.MBzID, Status: true,
				XXPrimaryTypes: "Album,EP",
			}))
		}
		// a2 only watches EPs.
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{
			UserID: user.ID, ArtistMBzID: "a2", Status: true, XXPrimaryTypes: "EP",
		}))

		future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", PrimaryType: "Album", ReleaseDate: "2000-01-01", LibraryStatus: LibraryStatusPresent}, a1, a2)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r2", PrimaryType: "Album", ReleaseDate: "2001-01-01", LibraryStatus: LibraryStatusMissing}, a1)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r3", PrimaryType: "Album", ReleaseDate: future, LibraryStatus: LibraryStatusMissing}, a1)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r4", PrimaryType: "EP", ReleaseDate: "2002-01-01", LibraryStatus: LibraryStatusMissing}, a2)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r5", PrimaryType: "EP", ReleaseDate: future}, a2)

		res, err := db.UserWatchedArtistsWithStats(ctx, user)
		require.NoError(t, err)
		require.Len(t, res, 2)

		require.Equal(t, "a1", res[0].MBzID)
		require.Equal(t, 1, res[0].Present)
		require.Equal(t, 1, res[0].Missing)
		require.Equal(t, 1, res[0].Upcoming)

		require.Equal(t, "a2", res[1].MBzID)
		require.Equal(t, 0, res[1].Present) // r1 is an Album, filtered out
		require.Equal(t, 1, res[1].Missing)
		require.Equal(t, 1, res[1].Upcoming)
	})
}

func TestNotifiedReleaseGroupIDs(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		got, err := db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Empty(t, got)

		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r2", "u1"))
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r3", "u2"))

		got, err = db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Equal(t, map[string]struct{}{"r1": {}, "r2": {}}, got)
	})
}

func TestRemoveReleaseGroupArtistLastArtist(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		a1 := seedArtist(t, ctx, db, "a1")
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", ReleaseDate: "2020"}, a1)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r2", ReleaseDate: "2021"}, a1)
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r2", "u1"))

		require.NoError(t, db.RemoveReleaseGroupArtist(ctx, "r1", "a1"))

		_, err := db.ReleaseGroup(ctx, "r1")
		require.ErrorIs(t, err, ErrReleaseGroupNotFound)
		rgs, err := db.ArtistReleaseGroups(ctx, a1)
		require.NoError(t, err)
		require.Len(t, rgs, 1)
		require.Equal(t, "r2", rgs[0].MBzID)
		got, err := db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Equal(t, map[string]struct{}{"r2": {}}, got)
	})
}

func TestRemoveReleaseGroupArtistShared(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		a1 := seedArtist(t, ctx, db, "a1")
		a2 := seedArtist(t, ctx, db, "a2")
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", ReleaseDate: "2020"}, a1, a2)
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))
		cr := []CachedRelease{{MBzID: "x", RecordingIDs: []string{"y"}}}
		require.NoError(t, db.ReplaceReleaseGroupReleases(ctx, "r1", cr, time.Now()))

		require.NoError(t, db.RemoveReleaseGroupArtist(ctx, "r1", "a1"))

		_, err := db.ReleaseGroup(ctx, "r1")
		require.NoError(t, err)
		rgs, err := db.ArtistReleaseGroups(ctx, a1)
		require.NoError(t, err)
		require.Empty(t, rgs)
		rgs, err = db.ArtistReleaseGroups(ctx, a2)
		require.NoError(t, err)
		require.Len(t, rgs, 1)
		got, err := db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Equal(t, map[string]struct{}{"r1": {}}, got)
		cached, err := db.ReleaseGroupReleases(ctx, "r1")
		require.NoError(t, err)
		require.Len(t, cached, 1)

		// removing the last artist drops the group
		require.NoError(t, db.RemoveReleaseGroupArtist(ctx, "r1", "a2"))
		_, err = db.ReleaseGroup(ctx, "r1")
		require.ErrorIs(t, err, ErrReleaseGroupNotFound)
		got, err = db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

func TestDeleteReleaseGroupNotification(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u2"))
		require.NoError(t, db.DeleteReleaseGroupNotification(ctx, "r1", "u1"))

		got, err := db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Empty(t, got)
		got, err = db.NotifiedReleaseGroupIDs(ctx, "u2")
		require.NoError(t, err)
		require.Len(t, got, 1)
	})
}

func seedUsers(t *testing.T, ctx context.Context, db *DB, ids ...string) {
	for _, id := range ids {
		require.NoError(t, db.CreateUser(ctx, User{ID: id, Name: id}))
	}
}
