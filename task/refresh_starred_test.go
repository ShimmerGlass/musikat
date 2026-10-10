package task

import (
	"context"
	"testing"
	"time"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/subsonic"
	"github.com/stretchr/testify/require"
)

func TestRefreshStarredCreatesWatch(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	e.sub.set(nil, []subArtist{
		{ID: "s1", Name: "Artist", MBzID: "a1"},
		{ID: "s2", Name: "Other", MBzID: "a2"},
	})

	before := time.Now().Unix()
	require.NoError(t, NewRefreshStarred(e.db, e.subClient).Run(ctx))

	for _, id := range []string{"a1", "a2"} {
		w, ok, err := e.db.ArtistWatch(ctx, "u1", id)
		require.NoError(t, err)
		require.True(t, ok, id)
		require.Equal(t, "subsonic", w.Source)
		require.True(t, w.Status)
		require.Equal(t, "Album,EP,Single", w.XXPrimaryTypes)
		require.Empty(t, w.XXSecondaryTypes)
		require.GreaterOrEqual(t, w.AddedAt, before)
		require.LessOrEqual(t, w.AddedAt, time.Now().Unix())
	}

	a, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.Equal(t, "Artist", a.Name)
}

func TestRefreshStarredKeepsExistingWatch(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	putArtist(t, e.db, database.Artist{MBzID: "a1", Name: "Custom Name"})
	require.NoError(t, e.db.AddArtistWatch(ctx, database.ArtistWatch{
		UserID: "u1", ArtistMBzID: "a1", Source: "manual", Status: true, AddedAt: 42,
		XXPrimaryTypes: "Album", XXSecondaryTypes: "Live",
	}))
	e.sub.set(nil, []subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}})

	require.NoError(t, NewRefreshStarred(e.db, e.subClient).Run(ctx))

	w, ok, err := e.db.ArtistWatch(ctx, "u1", "a1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, database.ArtistWatch{
		UserID: "u1", ArtistMBzID: "a1", Source: "manual", Status: true, AddedAt: 42,
		XXPrimaryTypes: "Album", XXSecondaryTypes: "Live",
	}, w)
	a, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.Equal(t, "Custom Name", a.Name)
}

func TestRefreshStarredDoesNotReenableDisabledWatch(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	putArtist(t, e.db, artistA)
	require.NoError(t, e.db.AddArtistWatch(ctx, database.ArtistWatch{
		UserID: "u1", ArtistMBzID: "a1", Source: "subsonic", Status: false, XXPrimaryTypes: "Album",
	}))
	e.sub.set(nil, []subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}})

	require.NoError(t, NewRefreshStarred(e.db, e.subClient).Run(ctx))

	w, ok, err := e.db.ArtistWatch(ctx, "u1", "a1")
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, w.Status)
}

func TestRefreshStarredPerUserAndIdempotent(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	e.addUser(t, "u2")
	require.NoError(t, e.db.CreateUser(ctx, database.User{ID: "u3", Name: "nosub"}))
	e.sub.set(nil, []subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}})
	task := NewRefreshStarred(e.db, e.subClient)

	require.NoError(t, task.Run(ctx))
	w1, _, err := e.db.ArtistWatch(ctx, "u1", "a1")
	require.NoError(t, err)

	require.NoError(t, task.Run(ctx))
	w1b, _, err := e.db.ArtistWatch(ctx, "u1", "a1")
	require.NoError(t, err)
	require.Equal(t, w1, w1b)

	for _, u := range []string{"u1", "u2"} {
		_, ok, err := e.db.ArtistWatch(ctx, u, "a1")
		require.NoError(t, err)
		require.True(t, ok, u)
	}
	_, ok, err := e.db.ArtistWatch(ctx, "u3", "a1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestRefreshStarredNoStarredArtists(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")

	require.NoError(t, NewRefreshStarred(e.db, e.subClient).Run(ctx))

	artists, err := e.db.Artists(ctx)
	require.NoError(t, err)
	require.Empty(t, artists)
}

func TestRefreshStarredNoUsersIsNoop(t *testing.T) {
	require.NoError(t, NewRefreshStarred(newTestDB(t), subsonic.New(subsonic.Config{})).Run(context.Background()))
}

func TestRefreshStarredPartialCredentialsDoNotBreakOtherUsers(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	require.NoError(t, e.db.CreateUser(ctx, database.User{ID: "a-half", Name: "half", SubsonicUser: "x"}))
	e.addUser(t, "b-ok")
	e.sub.set(nil, []subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}})

	require.NoError(t, NewRefreshStarred(e.db, e.subClient).Run(ctx))

	_, ok, err := e.db.ArtistWatch(ctx, "b-ok", "a1")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestRefreshStarredIgnoresArtistsWithoutMusicBrainzID(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	e.sub.set(nil, []subArtist{{ID: "s2", Name: "Untagged"}})

	require.NoError(t, NewRefreshStarred(e.db, e.subClient).Run(ctx))

	artists, err := e.db.Artists(ctx)
	require.NoError(t, err)
	require.Empty(t, artists)
}
