package database

import (
	"context"
	"testing"
	"time"

	opt "github.com/shimmerglass/go-optional"
	"github.com/stretchr/testify/require"
)

func TestArtistHelpers(t *testing.T) {
	a := Artist{MBzID: "abc-123", Name: "X"}
	require.Equal(t, "https://musicbrainz.org/artist/abc-123", a.MBzURL())
	require.Equal(t, "/artists/abc-123", a.UIPath())
}

func TestArtistNotFound(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		got, err := db.Artist(ctx, "missing")
		require.ErrorIs(t, err, ErrArtistNotFound)
		require.Equal(t, Artist{}, got)
	})
}

func TestPutArtist(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		// create: callback receives None
		called := 0
		err := db.PutArtist(ctx, "a1", func(o opt.Option[Artist]) Artist {
			called++
			require.True(t, o.IsNone())
			return Artist{MBzID: "a1", Name: "First"}
		})
		require.NoError(t, err)
		require.Equal(t, 1, called)

		got, err := db.Artist(ctx, "a1")
		require.NoError(t, err)
		require.Equal(t, Artist{MBzID: "a1", Name: "First"}, got)
		require.Nil(t, got.RefreshedAt)

		// update: callback receives Some(existing)
		ts := int64(12345)
		err = db.PutArtist(ctx, "a1", func(o opt.Option[Artist]) Artist {
			require.True(t, o.IsSome())
			existing := o.Unwrap()
			require.Equal(t, "First", existing.Name)
			existing.Name = "Second"
			existing.RefreshedAt = &ts
			return existing
		})
		require.NoError(t, err)

		got, err = db.Artist(ctx, "a1")
		require.NoError(t, err)
		require.Equal(t, "Second", got.Name)
		require.NotNil(t, got.RefreshedAt)
		require.Equal(t, ts, *got.RefreshedAt)

		all, err := db.Artists(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
	})
}

func TestArtistsEmpty(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		all, err := db.Artists(ctx)
		require.NoError(t, err)
		require.NotNil(t, all)
		require.Empty(t, all)
	})
}

func TestWatchedArtists(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1", "u2")
		for _, id := range []string{"a1", "a2", "a3", "a4"} {
			seedArtist(t, ctx, db, id)
		}
		watch := func(user, artist string, status bool) {
			require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: user, ArtistMBzID: artist, Status: status}))
		}

		got, err := db.WatchedArtists(ctx)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)

		watch("u1", "a1", true)
		watch("u2", "a1", true)  // duplicated across users
		watch("u1", "a2", false) // inactive
		watch("u2", "a3", true)
		watch("u1", "a3", false) // active for one user is enough
		// a4 has no watch

		got, err = db.WatchedArtists(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"a1", "a3"}, ids(got))
	})
}

func TestUserWatchedArtists(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1", "u2")
		u1 := User{ID: "u1"}
		for _, a := range []Artist{
			{MBzID: "a1", Name: "Charlie"},
			{MBzID: "a2", Name: "Alpha"},
			{MBzID: "a3", Name: "Bravo"},
			{MBzID: "a4", Name: "Delta"},
			{MBzID: "a5", Name: "Echo"},
		} {
			require.NoError(t, db.addArtist(ctx, a))
		}
		watch := func(user, artist string, status bool) {
			require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: user, ArtistMBzID: artist, Status: status}))
		}
		watch("u1", "a1", true)
		watch("u1", "a2", true)
		watch("u1", "a3", true)
		watch("u1", "a4", false) // inactive
		watch("u2", "a5", true)  // other user

		got, err := db.UserWatchedArtists(ctx, u1)
		require.NoError(t, err)
		// ordered by name
		require.Equal(t, []string{"a2", "a3", "a1"}, ids(got))

		got, err = db.UserWatchedArtists(ctx, User{ID: "nobody"})
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})
}

func TestUserWatchedArtistsWithStatsEdgeCases(t *testing.T) {
	future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")

	t.Run("no watched artists", func(t *testing.T) {
		WithDB(t, func(ctx context.Context, db *DB) {
			seedUsers(t, ctx, db, "u1", "u2")
			res, err := db.UserWatchedArtistsWithStats(ctx, User{ID: "u1"})
			require.NoError(t, err)
			require.NotNil(t, res)
			require.Empty(t, res)
		})
	})

	t.Run("artist without release groups has zero stats", func(t *testing.T) {
		WithDB(t, func(ctx context.Context, db *DB) {
			seedUsers(t, ctx, db, "u1", "u2")
			a := seedArtist(t, ctx, db, "a1")
			require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true, XXPrimaryTypes: "Album"}))

			res, err := db.UserWatchedArtistsWithStats(ctx, User{ID: "u1"})
			require.NoError(t, err)
			require.Len(t, res, 1)
			require.Equal(t, ArtistWithStats{Artist: a}, res[0])
		})
	})

	t.Run("inactive watch and other users excluded", func(t *testing.T) {
		WithDB(t, func(ctx context.Context, db *DB) {
			seedUsers(t, ctx, db, "u1", "u2")
			a1 := seedArtist(t, ctx, db, "a1")
			a2 := seedArtist(t, ctx, db, "a2")
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", PrimaryType: "Album", ReleaseDate: "2000", LibraryStatus: LibraryStatusPresent}, a1, a2)
			require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: false, XXPrimaryTypes: "Album"}))
			require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u2", ArtistMBzID: "a2", Status: true, XXPrimaryTypes: "Album"}))

			res, err := db.UserWatchedArtistsWithStats(ctx, User{ID: "u1"})
			require.NoError(t, err)
			require.Empty(t, res)
		})
	})

	t.Run("type filtering with unknown primary and secondary types", func(t *testing.T) {
		WithDB(t, func(ctx context.Context, db *DB) {
			seedUsers(t, ctx, db, "u1", "u2")
			a1 := seedArtist(t, ctx, db, "a1")
			require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{
				UserID: "u1", ArtistMBzID: "a1", Status: true,
				XXPrimaryTypes: "Album,Unknown", XXSecondaryTypes: "Live",
			}))
			// counted: album without secondary
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", PrimaryType: "Album", ReleaseDate: "2000", LibraryStatus: LibraryStatusPresent}, a1)
			// counted: unknown primary type
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r2", PrimaryType: "", ReleaseDate: "2001", LibraryStatus: LibraryStatusPresent}, a1)
			// counted: secondary type matches
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r3", PrimaryType: "Album", XXSecondaryTypes: "Live", ReleaseDate: "2002", LibraryStatus: LibraryStatusPresent}, a1)
			// filtered: secondary type not watched
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r4", PrimaryType: "Album", XXSecondaryTypes: "Compilation", ReleaseDate: "2003", LibraryStatus: LibraryStatusPresent}, a1)
			// filtered: primary type not watched
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r5", PrimaryType: "Single", ReleaseDate: "2004", LibraryStatus: LibraryStatusPresent}, a1)

			res, err := db.UserWatchedArtistsWithStats(ctx, User{ID: "u1"})
			require.NoError(t, err)
			require.Len(t, res, 1)
			require.Equal(t, 3, res[0].Present)
			require.Equal(t, 0, res[0].Missing)
			require.Equal(t, 0, res[0].Upcoming)
		})
	})

	t.Run("library statuses", func(t *testing.T) {
		WithDB(t, func(ctx context.Context, db *DB) {
			seedUsers(t, ctx, db, "u1", "u2")
			a1 := seedArtist(t, ctx, db, "a1")
			require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true, XXPrimaryTypes: "Album"}))
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", PrimaryType: "Album", ReleaseDate: "2000", LibraryStatus: LibraryStatusPresent}, a1)
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r2", PrimaryType: "Album", ReleaseDate: "2001", LibraryStatus: LibraryStatusMissing}, a1)
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r3", PrimaryType: "Album", ReleaseDate: "2002", LibraryStatus: LibraryStatusUnknown}, a1)
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r4", PrimaryType: "Album", ReleaseDate: "2003", LibraryStatus: LibraryStatusSongsPresent}, a1)
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r5", PrimaryType: "Album", ReleaseDate: future, LibraryStatus: LibraryStatusUnknown}, a1)
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r6", PrimaryType: "Album", ReleaseDate: future, LibraryStatus: LibraryStatusMissing}, a1)

			res, err := db.UserWatchedArtistsWithStats(ctx, User{ID: "u1"})
			require.NoError(t, err)
			require.Len(t, res, 1)
			require.Equal(t, 1, res[0].Present)
			require.Equal(t, 1, res[0].Missing) // r6 is upcoming, not missing
			require.Equal(t, 2, res[0].Upcoming)
		})
	})

	t.Run("shared release group counted per artist and results ordered by name", func(t *testing.T) {
		WithDB(t, func(ctx context.Context, db *DB) {
			seedUsers(t, ctx, db, "u1", "u2")
			require.NoError(t, db.addArtist(ctx, Artist{MBzID: "a1", Name: "Zed"}))
			require.NoError(t, db.addArtist(ctx, Artist{MBzID: "a2", Name: "Abe"}))
			for _, id := range []string{"a1", "a2"} {
				require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: id, Status: true, XXPrimaryTypes: "Album"}))
			}
			seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", PrimaryType: "Album", ReleaseDate: "2000", LibraryStatus: LibraryStatusPresent},
				Artist{MBzID: "a1"}, Artist{MBzID: "a2"})

			res, err := db.UserWatchedArtistsWithStats(ctx, User{ID: "u1"})
			require.NoError(t, err)
			require.Len(t, res, 2)
			require.Equal(t, "a2", res[0].MBzID)
			require.Equal(t, "a1", res[1].MBzID)
			require.Equal(t, 1, res[0].Present)
			require.Equal(t, 1, res[1].Present)
		})
	})
}

func seedWatch(t *testing.T, ctx context.Context, db *DB, user, artist string, status bool) {
	require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: user, ArtistMBzID: artist, Status: status}))
}

func TestDueArtistsAndMarkChecked(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1", "u2")
		for _, id := range []string{"a1", "a2", "a3", "a4", "a5", "unwatched", "disabled"} {
			seedArtist(t, ctx, db, id)
		}
		seedWatch(t, ctx, db, "u1", "a1", true)
		seedWatch(t, ctx, db, "u2", "a1", true) // watched twice, returned once
		seedWatch(t, ctx, db, "u1", "a2", true)
		seedWatch(t, ctx, db, "u1", "a3", true)
		seedWatch(t, ctx, db, "u1", "a4", true)
		seedWatch(t, ctx, db, "u1", "a5", true)
		seedWatch(t, ctx, db, "u1", "disabled", false)

		base := time.Unix(1_000_000, 0)
		require.NoError(t, db.MarkArtistsChecked(ctx, []string{"a3"}, base.Add(-3*time.Hour)))
		require.NoError(t, db.MarkArtistsChecked(ctx, []string{"a2"}, base.Add(-5*time.Hour)))
		require.NoError(t, db.MarkArtistsChecked(ctx, []string{"a4"}, base)) // fresh
		require.NoError(t, db.MarkArtistsChecked(ctx, []string{"a5"}, base.Add(-time.Hour)))

		// never checked first (by id), then oldest first; a4 is not due
		due, err := db.DueArtists(ctx, base.Add(-30*time.Minute), 10)
		require.NoError(t, err)
		require.Equal(t, []string{"a1", "a2", "a3", "a5"}, ids(due))

		due, err = db.DueArtists(ctx, base.Add(-30*time.Minute), 2)
		require.NoError(t, err)
		require.Equal(t, []string{"a1", "a2"}, ids(due))

		// strictly older than the threshold
		due, err = db.DueArtists(ctx, base.Add(-time.Hour), 10)
		require.NoError(t, err)
		require.Equal(t, []string{"a1", "a2", "a3"}, ids(due))

		n, err := db.CountDueArtists(ctx, base.Add(-30*time.Minute))
		require.NoError(t, err)
		require.Equal(t, 4, n)
		n, err = db.CountDueArtists(ctx, base.Add(-time.Hour))
		require.NoError(t, err)
		require.Equal(t, 3, n)

		due, err = db.DueArtists(ctx, base, 0)
		require.NoError(t, err)
		require.Empty(t, due)

		require.NoError(t, db.MarkArtistsChecked(ctx, nil, base))
		require.NoError(t, db.MarkArtistsChecked(ctx, []string{"a1", "a2"}, base))
		got, err := db.Artist(ctx, "a1")
		require.NoError(t, err)
		require.Equal(t, base.Unix(), *got.CheckedAt)
		require.Nil(t, got.RefreshedAt)
	})
}

func TestUpsertKeepsCheckedAt(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedArtist(t, ctx, db, "a1")
		at := time.Unix(5000, 0)
		require.NoError(t, db.MarkArtistsChecked(ctx, []string{"a1"}, at))

		// a stale value without checked_at, as written by other tasks
		require.NoError(t, db.addArtist(ctx, Artist{MBzID: "a1", Name: "Renamed"}))
		require.NoError(t, db.PutArtist(ctx, "a1", func(opt.Option[Artist]) Artist {
			return Artist{MBzID: "a1", Name: "Again", RefreshedAt: new(int64(7))}
		}))

		got, err := db.Artist(ctx, "a1")
		require.NoError(t, err)
		require.Equal(t, "Again", got.Name)
		require.Equal(t, int64(7), *got.RefreshedAt)
		require.NotNil(t, got.CheckedAt)
		require.Equal(t, at.Unix(), *got.CheckedAt)
	})
}
