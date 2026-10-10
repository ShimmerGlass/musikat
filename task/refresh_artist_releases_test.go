package task

import (
	"context"
	"testing"

	opt "github.com/shimmerglass/go-optional"
	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
)

func putArtist(t *testing.T, db *database.DB, a database.Artist) {
	require.NoError(t, db.PutArtist(context.Background(), a.MBzID, func(opt.Option[database.Artist]) database.Artist { return a }))
}

func putRG(t *testing.T, db *database.DB, rg database.ReleaseGroup) {
	ctx := context.Background()
	for _, a := range rg.Artists {
		putArtist(t, db, a)
	}
	require.NoError(t, db.PutReleaseGroup(ctx, rg.MBzID, func(opt.Option[database.ReleaseGroup]) database.ReleaseGroup { return rg }))
	require.NoError(t, db.ReplaceReleaseGroupArtists(ctx, rg))
}

func artistRGIDs(t *testing.T, db *database.DB, artistID string) []string {
	rgs, err := db.ArtistReleaseGroups(context.Background(), database.Artist{MBzID: artistID})
	require.NoError(t, err)
	ids := []string{}
	for _, rg := range rgs {
		ids = append(ids, rg.MBzID)
	}
	return ids
}

var (
	artistA = database.Artist{MBzID: "a1", Name: "Artist"}
	artistB = database.Artist{MBzID: "a2", Name: "Other"}
)

func TestRefreshArtistReleasesStoresNewReleaseGroups(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	putArtist(t, e.db, artistA)
	e.mb.rgs["a1"] = []mbRG{
		{ID: "rg1", Title: "First", Primary: "Album", Date: "2020-05-01", Artists: []database.Artist{artistA, artistB}},
		{ID: "rg2", Title: "Live", Primary: "Album", Secondary: []string{"Live"}, Date: "2021", Artists: []database.Artist{artistA}},
		{ID: "rg3", Title: "Best Of", Primary: "Album", Secondary: []string{"Compilation"}, Date: "2022", Artists: []database.Artist{artistA}},
	}

	task := NewRefreshArtistReleases(e.db, e.mbClient)
	require.NoError(t, task.RunArtist(ctx, artistA))

	require.ElementsMatch(t, []string{"rg1", "rg2"}, artistRGIDs(t, e.db, "a1"))

	rg1, err := e.db.ReleaseGroup(ctx, "rg1")
	require.NoError(t, err)
	require.Equal(t, "First", rg1.Name)
	require.Equal(t, "Album", rg1.PrimaryType)
	require.Equal(t, "2020-05-01", rg1.ReleaseDate)

	rg2, err := e.db.ReleaseGroup(ctx, "rg2")
	require.NoError(t, err)
	require.Equal(t, "Live", rg2.XXSecondaryTypes)
	require.Equal(t, "2021", rg2.ReleaseDate)

	// credited co-artists are stored and linked
	b, err := e.db.Artist(ctx, "a2")
	require.NoError(t, err)
	require.Equal(t, "Other", b.Name)
	require.Equal(t, []string{"rg1"}, artistRGIDs(t, e.db, "a2"))

	// compilations are skipped entirely
	_, err = e.db.ReleaseGroup(ctx, "rg3")
	require.ErrorIs(t, err, database.ErrReleaseGroupNotFound)
}

func TestRefreshArtistReleasesPaginates(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.mb.pageSize = 2
	putArtist(t, e.db, artistA)
	for _, id := range []string{"rg1", "rg2", "rg3", "rg4", "rg5"} {
		e.mb.rgs["a1"] = append(e.mb.rgs["a1"], mbRG{ID: id, Title: id, Primary: "Album", Date: "2020", Artists: []database.Artist{artistA}})
	}

	require.NoError(t, NewRefreshArtistReleases(e.db, e.mbClient).RunArtist(ctx, artistA))
	require.ElementsMatch(t, []string{"rg1", "rg2", "rg3", "rg4", "rg5"}, artistRGIDs(t, e.db, "a1"))
	require.Equal(t, 3, e.mb.requestCount())
}

func TestRefreshArtistReleasesPreservesLibraryState(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	fetched := int64(12345)
	putRG(t, e.db, database.ReleaseGroup{
		MBzID: "rg1", Name: "Old Name", PrimaryType: "EP", ReleaseDate: "2019",
		LibraryStatus: database.LibraryStatusPresent, InLibraryReleaseMBzID: "rel9",
		ReleasesFetchedAt: &fetched,
		Artists:           []database.Artist{artistA},
	})
	e.mb.rgs["a1"] = []mbRG{
		{ID: "rg1", Title: "New Name", Primary: "Album", Secondary: []string{"Live"}, Date: "2019-03-02", Artists: []database.Artist{artistA}},
	}

	require.NoError(t, NewRefreshArtistReleases(e.db, e.mbClient).RunArtist(ctx, artistA))

	rg, err := e.db.ReleaseGroup(ctx, "rg1")
	require.NoError(t, err)
	require.Equal(t, "New Name", rg.Name)
	require.Equal(t, "Album", rg.PrimaryType)
	require.Equal(t, "Live", rg.XXSecondaryTypes)
	require.Equal(t, "2019-03-02", rg.ReleaseDate)
	require.Equal(t, database.LibraryStatusPresent, rg.LibraryStatus)
	require.Equal(t, "rel9", rg.InLibraryReleaseMBzID)
	require.NotNil(t, rg.ReleasesFetchedAt)
	require.Equal(t, fetched, *rg.ReleasesFetchedAt)
}

func TestRefreshArtistReleasesUnlinksOnlyThisArtist(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	both := []database.Artist{artistA, artistB}
	putRG(t, e.db, database.ReleaseGroup{MBzID: "shared", Name: "Shared", PrimaryType: "Album", Artists: both})
	putRG(t, e.db, database.ReleaseGroup{MBzID: "gone", Name: "Gone", PrimaryType: "Album", Artists: both})
	putRG(t, e.db, database.ReleaseGroup{MBzID: "kept", Name: "Kept", PrimaryType: "Album", Artists: both})

	// MusicBrainz still lists "shared" and "kept" for a1 (with both credited),
	// but "gone" no longer belongs to a1.
	e.mb.rgs["a1"] = []mbRG{
		{ID: "shared", Title: "Shared", Primary: "Album", Artists: both},
		{ID: "kept", Title: "Kept", Primary: "Album", Artists: both},
	}

	require.NoError(t, NewRefreshArtistReleases(e.db, e.mbClient).RunArtist(ctx, artistA))

	require.ElementsMatch(t, []string{"shared", "kept"}, artistRGIDs(t, e.db, "a1"))
	// the other artist keeps every release group, including the one removed from a1
	require.ElementsMatch(t, []string{"shared", "gone", "kept"}, artistRGIDs(t, e.db, "a2"))

	// the release group itself is not deleted
	_, err := e.db.ReleaseGroup(ctx, "gone")
	require.NoError(t, err)
}

func TestRefreshArtistReleasesRunOnlyWatchedArtists(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	putArtist(t, e.db, artistA)
	putArtist(t, e.db, artistB)
	require.NoError(t, e.db.AddArtistWatch(ctx, database.ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true, XXPrimaryTypes: "Album"}))
	require.NoError(t, e.db.AddArtistWatch(ctx, database.ArtistWatch{UserID: "u1", ArtistMBzID: "a2", Status: false, XXPrimaryTypes: "Album"}))
	e.mb.rgs["a1"] = []mbRG{{ID: "rg1", Title: "One", Primary: "Album", Artists: []database.Artist{artistA}}}
	e.mb.rgs["a2"] = []mbRG{{ID: "rg2", Title: "Two", Primary: "Album", Artists: []database.Artist{artistB}}}

	require.NoError(t, NewRefreshArtistReleases(e.db, e.mbClient).Run(ctx))

	require.Equal(t, []string{"rg1"}, artistRGIDs(t, e.db, "a1"))
	require.Empty(t, artistRGIDs(t, e.db, "a2"))
}

func TestRefreshArtistReleasesMusicBrainzErrorKeepsExisting(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	putRG(t, e.db, database.ReleaseGroup{MBzID: "rg1", Name: "One", PrimaryType: "Album", Artists: []database.Artist{artistA}})
	e.mb.Close() // every request now fails

	err := NewRefreshArtistReleases(e.db, e.mbClient).RunArtist(ctx, artistA)
	require.Error(t, err)
	require.Equal(t, []string{"rg1"}, artistRGIDs(t, e.db, "a1"))
}
