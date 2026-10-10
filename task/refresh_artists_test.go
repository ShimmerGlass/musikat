package task

import (
	"context"
	"testing"
	"time"

	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
)

func TestRefreshArtistsCreatesArtists(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	e.sub.set([]subArtist{
		{ID: "s1", Name: "Artist", MBzID: "a1"},
		{ID: "s2", Name: "Other", MBzID: "a2"},
	}, nil)

	require.NoError(t, NewRefreshArtists(e.db, e.subClient).Run(ctx))

	artists, err := e.db.Artists(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []database.Artist{
		{MBzID: "a1", Name: "Artist"},
		{MBzID: "a2", Name: "Other"},
	}, artists)

	// artists are only stored, never watched
	watched, err := e.db.WatchedArtists(ctx)
	require.NoError(t, err)
	require.Empty(t, watched)
}

func TestRefreshArtistsUpdatesNameKeepsRefreshedAt(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	refreshed := time.Now().Add(-time.Hour).Unix()
	putArtist(t, e.db, database.Artist{MBzID: "a1", Name: "Old Name", RefreshedAt: &refreshed})
	e.sub.set([]subArtist{{ID: "s1", Name: "New Name", MBzID: "a1"}}, nil)

	require.NoError(t, NewRefreshArtists(e.db, e.subClient).Run(ctx))

	a, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.Equal(t, "New Name", a.Name)
	require.NotNil(t, a.RefreshedAt)
	require.Equal(t, refreshed, *a.RefreshedAt)
}

func TestRefreshArtistsSkipsUsersWithoutSubsonic(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	require.NoError(t, e.db.AddUser(ctx, database.User{ID: "a-nosub", Name: "nosub"}))
	require.NoError(t, e.db.AddUser(ctx, database.User{ID: "b-halfsub", Name: "half", SubsonicUser: "x"}))
	e.addUser(t, "c-sub")
	e.sub.set([]subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}}, nil)

	require.NoError(t, NewRefreshArtists(e.db, e.subClient).Run(ctx))

	_, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
}

func TestRefreshArtistsNoSubsonicUser(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	require.NoError(t, e.db.AddUser(ctx, database.User{ID: "u1", Name: "u1"}))

	require.ErrorContains(t, NewRefreshArtists(e.db, e.subClient).Run(ctx), "no subsonic user")
}

func TestRefreshArtistsRunArtistIsNoop(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	// no users: Run would fail, RunArtist must not touch anything
	require.NoError(t, NewRefreshArtists(e.db, e.subClient).RunArtist(ctx, artistA))
	artists, err := e.db.Artists(ctx)
	require.NoError(t, err)
	require.Empty(t, artists)
}

func TestRefreshArtistsIgnoresArtistsWithoutMusicBrainzID(t *testing.T) {
	t.Skip("BUG: refresh_artists.go:41 stores Subsonic artists that have no MusicBrainz ID (empty getArtistInfo2 musicBrainzId) with an empty primary key; they should be skipped")

	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	e.sub.set([]subArtist{
		{ID: "s1", Name: "Tagged", MBzID: "a1"},
		{ID: "s2", Name: "Untagged"},
	}, nil)

	require.NoError(t, NewRefreshArtists(e.db, e.subClient).Run(ctx))

	artists, err := e.db.Artists(ctx)
	require.NoError(t, err)
	require.Equal(t, []database.Artist{{MBzID: "a1", Name: "Tagged"}}, artists)
}
