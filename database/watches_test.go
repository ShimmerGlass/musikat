package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtistWatchEmptyTypesAreNil(t *testing.T) {
	w := ArtistWatch{}
	require.Nil(t, w.PrimaryTypes())
	require.Nil(t, w.SecondaryTypes())
}

func TestAddArtistWatchForeignKeys(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1")
		seedArtist(t, ctx, db, "a1")

		require.Error(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "ghost", Status: true}), "unknown artist")
		require.Error(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "nobody", ArtistMBzID: "a1", Status: true}), "unknown user")
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true}))

		watches, err := db.UserArtistWatches(ctx, User{ID: "u1"})
		require.NoError(t, err)
		require.Len(t, watches, 1)
	})
}

func TestAddArtistWatchHonorsContext(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1")
		seedArtist(t, ctx, db, "a1")

		cctx, cancel := context.WithCancel(ctx)
		cancel()
		require.Error(t, db.AddArtistWatch(cctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1"}))
	})
}

func TestArtistWatchTypeSplit(t *testing.T) {
	w := ArtistWatch{XXPrimaryTypes: "Album,EP", XXSecondaryTypes: "Live,Remix"}
	require.Equal(t, []string{"Album", "EP"}, w.PrimaryTypes())
	require.Equal(t, []string{"Live", "Remix"}, w.SecondaryTypes())
}

func TestArtistWatchWatches(t *testing.T) {
	for _, tc := range []struct {
		name    string
		primary string
		second  string
		rg      ReleaseGroup
		want    bool
	}{
		{"primary match no secondary", "Album,EP", "", ReleaseGroup{PrimaryType: "Album"}, true},
		{"primary mismatch", "Album,EP", "", ReleaseGroup{PrimaryType: "Single"}, false},
		{"second primary in list", "Album,EP", "", ReleaseGroup{PrimaryType: "EP"}, true},
		{"unknown primary watched", "Album,Unknown", "", ReleaseGroup{PrimaryType: ""}, true},
		{"unknown primary not watched", "Album,EP", "", ReleaseGroup{PrimaryType: ""}, false},
		{"explicit Unknown string", "Unknown", "", ReleaseGroup{PrimaryType: "Unknown"}, true},
		{"empty watch types", "", "", ReleaseGroup{PrimaryType: "Album"}, false},
		{"secondary intersect", "Album", "Live,Remix", ReleaseGroup{PrimaryType: "Album", XXSecondaryTypes: "Live"}, true},
		{"secondary one of many intersects", "Album", "Live", ReleaseGroup{PrimaryType: "Album", XXSecondaryTypes: "Compilation,Live"}, true},
		{"secondary disjoint", "Album", "Live", ReleaseGroup{PrimaryType: "Album", XXSecondaryTypes: "Compilation"}, false},
		{"secondary but watch has none", "Album", "", ReleaseGroup{PrimaryType: "Album", XXSecondaryTypes: "Live"}, false},
		{"no rg secondary ignores watch secondary", "Album", "Live", ReleaseGroup{PrimaryType: "Album"}, true},
		{"secondary match wrong primary", "EP", "Live", ReleaseGroup{PrimaryType: "Album", XXSecondaryTypes: "Live"}, false},
		{"unknown primary with secondary", "Unknown", "Live", ReleaseGroup{XXSecondaryTypes: "Live"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := ArtistWatch{XXPrimaryTypes: tc.primary, XXSecondaryTypes: tc.second}
			require.Equal(t, tc.want, w.Watches(tc.rg))
		})
	}
}

func TestAddArtistWatchUpsert(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.CreateUser(ctx, User{ID: "u1", Name: "u1"}))
		seedArtist(t, ctx, db, "a1")

		w := ArtistWatch{
			UserID: "u1", ArtistMBzID: "a1", Source: "manual", Status: true,
			AddedAt: 1000, XXPrimaryTypes: "Album", XXSecondaryTypes: "Live",
		}
		require.NoError(t, db.AddArtistWatch(ctx, w))

		got, ok, err := db.ArtistWatch(ctx, "u1", "a1")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, w, got)

		// upsert changes status and types, keeps a single row
		w2 := w
		w2.Status = false
		w2.XXPrimaryTypes = "EP,Single"
		w2.XXSecondaryTypes = ""
		require.NoError(t, db.AddArtistWatch(ctx, w2))

		got, ok, err = db.ArtistWatch(ctx, "u1", "a1")
		require.NoError(t, err)
		require.True(t, ok)
		require.False(t, got.Status)
		require.Equal(t, "EP,Single", got.XXPrimaryTypes)
		require.Equal(t, "", got.XXSecondaryTypes)

		watches, err := db.UserArtistWatches(ctx, User{ID: "u1"})
		require.NoError(t, err)
		require.Len(t, watches, 1)
	})
}

func TestArtistWatchNotFound(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1", "u2")
		got, ok, err := db.ArtistWatch(ctx, "u1", "a1")
		require.NoError(t, err)
		require.False(t, ok)
		require.Equal(t, ArtistWatch{UserID: "u1", ArtistMBzID: "a1"}, got)

		seedArtist(t, ctx, db, "a1")
		seedArtist(t, ctx, db, "a2")

		// a watch for another user/artist does not match
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u2", ArtistMBzID: "a1", Status: true}))
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a2", Status: true}))
		_, ok, err = db.ArtistWatch(ctx, "u1", "a1")
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestSameArtistDifferentUsersWatchIndependent(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1", "u2")
		seedArtist(t, ctx, db, "a1")
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true, XXPrimaryTypes: "Album"}))
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u2", ArtistMBzID: "a1", Status: false, XXPrimaryTypes: "EP"}))

		w1, ok, err := db.ArtistWatch(ctx, "u1", "a1")
		require.NoError(t, err)
		require.True(t, ok)
		require.True(t, w1.Status)
		require.Equal(t, "Album", w1.XXPrimaryTypes)

		w2, ok, err := db.ArtistWatch(ctx, "u2", "a1")
		require.NoError(t, err)
		require.True(t, ok)
		require.False(t, w2.Status)
		require.Equal(t, "EP", w2.XXPrimaryTypes)
	})
}

func TestUserArtistWatches(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		seedUsers(t, ctx, db, "u1", "u2")
		got, err := db.UserArtistWatches(ctx, User{ID: "u1"})
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)

		for _, id := range []string{"a1", "a2", "a3"} {
			seedArtist(t, ctx, db, id)
		}
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true}))
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u1", ArtistMBzID: "a2", Status: false}))
		require.NoError(t, db.AddArtistWatch(ctx, ArtistWatch{UserID: "u2", ArtistMBzID: "a3", Status: true}))

		got, err = db.UserArtistWatches(ctx, User{ID: "u1"})
		require.NoError(t, err)
		require.Len(t, got, 2)
		byArtist := map[string]bool{}
		for _, w := range got {
			require.Equal(t, "u1", w.UserID)
			byArtist[w.ArtistMBzID] = w.Status
		}
		// inactive watches are included
		require.Equal(t, map[string]bool{"a1": true, "a2": false}, byArtist)
	})
}
