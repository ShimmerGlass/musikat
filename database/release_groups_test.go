package database

import (
	"context"
	"testing"

	opt "github.com/shimmerglass/go-optional"
	"github.com/stretchr/testify/require"
)

func TestReleaseGroupYear(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2020-02-12", "2020"},
		{"2020-02", "2020"},
		{"1998", "1998"},
		{"2005-??-31", "2005"},
		{"", ""},
	} {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, ReleaseGroup{ReleaseDate: tc.in}.Year())
		})
	}
}

func TestReleaseGroupMBzURL(t *testing.T) {
	rg := ReleaseGroup{MBzID: "abc-123"}
	require.Equal(t, "https://musicbrainz.org/release-group/abc-123", rg.MBzURL())
}

func TestReleaseGroupSecondaryTypes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"Live", []string{"Live"}},
		{"Live,Compilation", []string{"Live", "Compilation"}},
	} {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, ReleaseGroup{XXSecondaryTypes: tc.in}.SecondaryTypes())
		})
	}
}

func TestReleaseGroupNotFound(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		got, err := db.ReleaseGroup(ctx, "missing")
		require.ErrorIs(t, err, ErrReleaseGroupNotFound)
		require.Equal(t, ReleaseGroup{}, got)
	})
}

func TestPutReleaseGroup(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		created := ReleaseGroup{
			MBzID: "r1", Name: "First", PrimaryType: "Album", XXSecondaryTypes: "Live",
			ReleaseDate: "2020-01-02", LibraryStatus: LibraryStatusMissing,
			InLibraryReleaseMBzID: "rel1",
		}
		err := db.PutReleaseGroup(ctx, "r1", func(o opt.Option[ReleaseGroup]) ReleaseGroup {
			require.True(t, o.IsNone())
			return created
		})
		require.NoError(t, err)

		got, err := db.ReleaseGroup(ctx, "r1")
		require.NoError(t, err)
		require.Equal(t, created, got)

		err = db.PutReleaseGroup(ctx, "r1", func(o opt.Option[ReleaseGroup]) ReleaseGroup {
			require.True(t, o.IsSome())
			existing := o.Unwrap()
			require.Equal(t, created, existing)
			existing.Name = "Second"
			existing.PrimaryType = "EP"
			existing.XXSecondaryTypes = ""
			existing.ReleaseDate = "2021"
			existing.LibraryStatus = LibraryStatusPresent
			existing.InLibraryReleaseMBzID = "rel2"
			return existing
		})
		require.NoError(t, err)

		got, err = db.ReleaseGroup(ctx, "r1")
		require.NoError(t, err)
		require.Equal(t, ReleaseGroup{
			MBzID: "r1", Name: "Second", PrimaryType: "EP", ReleaseDate: "2021",
			LibraryStatus: LibraryStatusPresent, InLibraryReleaseMBzID: "rel2",
		}, got)

		all, err := db.ReleaseGroups(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
	})
}

func TestReleaseGroupsEmpty(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		all, err := db.ReleaseGroups(ctx)
		require.NoError(t, err)
		require.NotNil(t, all)
		require.Empty(t, all)
	})
}

func TestReplaceReleaseGroupArtists(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		a1 := seedArtist(t, ctx, db, "a1")
		a2 := seedArtist(t, ctx, db, "a2")
		a3 := seedArtist(t, ctx, db, "a3")
		rg := seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", ReleaseDate: "2020"}, a1, a2)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r2", ReleaseDate: "2021"}, a1)

		artistsOf := func(a Artist) []string {
			rgs, err := db.ArtistReleaseGroupsWithoutArtists(ctx, a)
			require.NoError(t, err)
			r := []string{}
			for _, g := range rgs {
				r = append(r, g.MBzID)
			}
			return r
		}
		require.Equal(t, []string{"r2", "r1"}, artistsOf(a1))
		require.Equal(t, []string{"r1"}, artistsOf(a2))

		// replace: a1 dropped, a3 added, a2 kept; other group untouched
		rg.Artists = []Artist{a2, a3}
		require.NoError(t, db.ReplaceReleaseGroupArtists(ctx, rg))
		require.Equal(t, []string{"r2"}, artistsOf(a1))
		require.Equal(t, []string{"r1"}, artistsOf(a2))
		require.Equal(t, []string{"r1"}, artistsOf(a3))

		// duplicates in input are tolerated
		rg.Artists = []Artist{a3, a3}
		require.NoError(t, db.ReplaceReleaseGroupArtists(ctx, rg))
		require.Empty(t, artistsOf(a2))
		require.Equal(t, []string{"r1"}, artistsOf(a3))

		// empty list unlinks everyone but keeps the release group
		rg.Artists = nil
		require.NoError(t, db.ReplaceReleaseGroupArtists(ctx, rg))
		require.Empty(t, artistsOf(a3))
		_, err := db.ReleaseGroup(ctx, "r1")
		require.NoError(t, err)
	})
}

func TestReleaseGroupUpsertKeepsArtists(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		a1 := seedArtist(t, ctx, db, "a1")
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", Name: "old", ReleaseDate: "2020"}, a1)

		require.NoError(t, db.PutReleaseGroup(ctx, "r1", func(o opt.Option[ReleaseGroup]) ReleaseGroup {
			rg := o.Unwrap()
			rg.Name = "new"
			return rg
		}))

		rgs, err := db.ArtistReleaseGroups(ctx, a1)
		require.NoError(t, err)
		require.Len(t, rgs, 1)
		require.Equal(t, "new", rgs[0].Name)
		require.Equal(t, []string{"a1"}, ids(rgs[0].Artists))
	})
}

func TestArtistReleaseGroupsOrderingAndEmptyArtists(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		a1 := seedArtist(t, ctx, db, "a1")
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r1", ReleaseDate: "2001-05-01"}, a1)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r2", ReleaseDate: "2003-01-01"}, a1)
		seedRG(t, ctx, db, ReleaseGroup{MBzID: "r3", ReleaseDate: "2002-12-31"}, a1)

		rgs, err := db.ArtistReleaseGroups(ctx, a1)
		require.NoError(t, err)
		got := []string{}
		for _, rg := range rgs {
			got = append(got, rg.MBzID)
		}
		require.Equal(t, []string{"r2", "r3", "r1"}, got)
	})
}
