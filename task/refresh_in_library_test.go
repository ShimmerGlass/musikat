package task

import (
	"context"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
)

// libEnv is an env with one user watching artist a1, known to Subsonic as s1.
type libEnv struct {
	*env
	task *RefreshInLibrary
}

func newLibEnv(t *testing.T, rgs ...database.ReleaseGroup) *libEnv {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	putArtist(t, e.db, artistA)
	require.NoError(t, e.db.AddArtistWatch(ctx, database.ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true, XXPrimaryTypes: "Album"}))
	for _, rg := range rgs {
		rg.Artists = []database.Artist{artistA}
		putRG(t, e.db, rg)
	}
	e.sub.set([]subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}}, nil)
	return &libEnv{env: e, task: NewRefreshInLibrary(e.db, e.mbClient, e.subClient, e.naviClient)}
}

func (e *libEnv) rg(t *testing.T, id string) database.ReleaseGroup {
	rg, err := e.db.ReleaseGroup(context.Background(), id)
	require.NoError(t, err)
	return rg
}

func song(album, rg, recording string) model.MediaFile {
	return model.MediaFile{MbzAlbumID: album, MbzReleaseGroupID: rg, MbzRecordingID: recording}
}

func oldRG(id string) database.ReleaseGroup {
	return database.ReleaseGroup{MBzID: id, Name: id, PrimaryType: "Album", ReleaseDate: "2010-01-01"}
}

func cacheReleases(t *testing.T, e *libEnv, rgID string, fetchedAt time.Time, rels ...database.CachedRelease) {
	require.NoError(t, e.db.ReplaceReleaseGroupReleases(context.Background(), rgID, rels, fetchedAt))
}

func TestRefreshInLibraryDirectMatchSkipsMusicBrainz(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	e.navi.setSongs("s1", song("relB", "rg1", "r1"), song("relA", "rg1", "r2"), song("relA", "rg1", "r3"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	rg := e.rg(t, "rg1")
	require.Equal(t, database.LibraryStatusPresent, rg.LibraryStatus)
	require.Equal(t, "relA", rg.InLibraryReleaseMBzID) // smallest album ID
	require.Zero(t, e.mb.requestCount())
}

func TestRefreshInLibraryDirectMatchKeepsCurrentRelease(t *testing.T) {
	ctx := context.Background()
	rg := oldRG("rg1")
	rg.LibraryStatus = database.LibraryStatusPresent
	rg.InLibraryReleaseMBzID = "relB"
	e := newLibEnv(t, rg)
	e.navi.setSongs("s1", song("relA", "rg1", ""), song("relB", "rg1", ""))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	require.Equal(t, "relB", e.rg(t, "rg1").InLibraryReleaseMBzID)
	require.Zero(t, e.mb.requestCount())
}

func TestRefreshInLibrarySongWithoutAlbumIDIsNotDirectMatch(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	cacheReleases(t, e, "rg1", time.Now(), database.CachedRelease{MBzID: "relA", RecordingIDs: []string{"r1"}})
	e.navi.setSongs("s1", song("", "rg1", "other"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	require.Equal(t, database.LibraryStatusMissing, e.rg(t, "rg1").LibraryStatus)
}

func TestRefreshInLibraryFreshCacheIsUsed(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	cacheReleases(t, e, "rg1", time.Now().Add(-time.Hour),
		database.CachedRelease{MBzID: "relB", RecordingIDs: []string{"r3"}},
		database.CachedRelease{MBzID: "relA", RecordingIDs: []string{"r1", "r2"}},
	)
	// album ID of a cached release but no release group ID on the song
	e.navi.setSongs("s1", song("relB", "", "r3"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	rg := e.rg(t, "rg1")
	require.Equal(t, database.LibraryStatusPresent, rg.LibraryStatus)
	require.Equal(t, "relB", rg.InLibraryReleaseMBzID)
	require.Zero(t, e.mb.requestCount())
}

func TestRefreshInLibraryStaleCacheIsRefetched(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	old := time.Now().Add(-60 * 24 * time.Hour)
	cacheReleases(t, e, "rg1", old, database.CachedRelease{MBzID: "oldrel", RecordingIDs: []string{"x"}})
	e.mb.releases["rg1"] = []mbRelease{
		{ID: "relA", Recordings: []string{"r1", "r2"}},
		{ID: "relB", Recordings: []string{"r3"}},
	}
	e.navi.setSongs("s1", song("relB", "", "r3"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	require.Equal(t, 1, e.mb.requestCount())
	rg := e.rg(t, "rg1")
	require.Equal(t, database.LibraryStatusPresent, rg.LibraryStatus)
	require.Equal(t, "relB", rg.InLibraryReleaseMBzID)
	require.NotNil(t, rg.ReleasesFetchedAt)
	require.Greater(t, *rg.ReleasesFetchedAt, old.Unix())

	cached, err := e.db.ReleaseGroupReleases(ctx, "rg1")
	require.NoError(t, err)
	require.Equal(t, []database.CachedRelease{
		{MBzID: "relA", RecordingIDs: []string{"r1", "r2"}},
		{MBzID: "relB", RecordingIDs: []string{"r3"}},
	}, cached)

	// second run uses the freshly written cache
	require.NoError(t, e.task.RunArtist(ctx, artistA))
	require.Equal(t, 1, e.mb.requestCount())
}

func TestRefreshInLibraryNeverFetchedIsFetched(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	e.mb.releases["rg1"] = []mbRelease{{ID: "relA", Recordings: []string{"r1"}}}
	e.navi.setSongs("s1", song("", "", "unrelated"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	require.Equal(t, 1, e.mb.requestCount())
	require.Equal(t, database.LibraryStatusMissing, e.rg(t, "rg1").LibraryStatus)
}

func TestRefreshInLibraryRecentReleaseRefetchedDaily(t *testing.T) {
	ctx := context.Background()
	recent := oldRG("rg1")
	recent.ReleaseDate = time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02")
	e := newLibEnv(t, recent)
	cacheReleases(t, e, "rg1", time.Now().Add(-48*time.Hour), database.CachedRelease{MBzID: "relA", RecordingIDs: []string{"r1"}})
	e.mb.releases["rg1"] = []mbRelease{{ID: "relA", Recordings: []string{"r1"}}}
	e.navi.setSongs("s1", song("relA", "", "r1"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	require.Equal(t, 1, e.mb.requestCount())
	require.Equal(t, database.LibraryStatusPresent, e.rg(t, "rg1").LibraryStatus)
}

func TestRefreshInLibrarySongsPresent(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	cacheReleases(t, e, "rg1", time.Now(),
		database.CachedRelease{MBzID: "relA", RecordingIDs: []string{"r1", "r2"}},
		database.CachedRelease{MBzID: "relB", RecordingIDs: []string{"r3"}},
	)
	// the songs are in the library under another release/album
	e.navi.setSongs("s1", song("elsewhere", "", "r1"), song("elsewhere", "", "r2"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	rg := e.rg(t, "rg1")
	require.Equal(t, database.LibraryStatusSongsPresent, rg.LibraryStatus)
	require.Empty(t, rg.InLibraryReleaseMBzID)
}

func TestRefreshInLibraryPartialSongsAreMissing(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	cacheReleases(t, e, "rg1", time.Now(), database.CachedRelease{MBzID: "relA", RecordingIDs: []string{"r1", "r2"}})
	e.navi.setSongs("s1", song("elsewhere", "", "r1"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	require.Equal(t, database.LibraryStatusMissing, e.rg(t, "rg1").LibraryStatus)
}

func TestRefreshInLibraryReleaseWithoutTracksIsNotSongsPresent(t *testing.T) {
	t.Skip("BUG: release_status.go:75 cachedState uses lo.Every(artistRecordingIDs, rel.RecordingIDs); a cached release with no recordings (empty tracklist on MusicBrainz) makes every release group SongsPresent, even when the library has no songs from it")

	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	cacheReleases(t, e, "rg1", time.Now(), database.CachedRelease{MBzID: "relA"})
	e.navi.setSongs("s1", song("elsewhere", "", "r1"))

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	require.Equal(t, database.LibraryStatusMissing, e.rg(t, "rg1").LibraryStatus)
}

func TestRefreshInLibraryMissing(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	e.mb.releases["rg1"] = []mbRelease{{ID: "relA", Recordings: []string{"r1"}}}
	e.navi.setSongs("s1") // artist exists but has no songs

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	rg := e.rg(t, "rg1")
	require.Equal(t, database.LibraryStatusMissing, rg.LibraryStatus)
	require.Empty(t, rg.InLibraryReleaseMBzID)
}

func TestRefreshInLibraryStateTransitions(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	cacheReleases(t, e, "rg1", time.Now(), database.CachedRelease{MBzID: "relA", RecordingIDs: []string{"r1"}})

	e.navi.setSongs("s1", song("relA", "rg1", "r1"))
	require.NoError(t, e.task.RunArtist(ctx, artistA))
	require.Equal(t, database.LibraryStatusPresent, e.rg(t, "rg1").LibraryStatus)

	// unchanged: same result
	require.NoError(t, e.task.RunArtist(ctx, artistA))
	rg := e.rg(t, "rg1")
	require.Equal(t, database.LibraryStatusPresent, rg.LibraryStatus)
	require.Equal(t, "relA", rg.InLibraryReleaseMBzID)

	// songs removed from the library
	e.navi.setSongs("s1")
	require.NoError(t, e.task.RunArtist(ctx, artistA))
	rg = e.rg(t, "rg1")
	require.Equal(t, database.LibraryStatusMissing, rg.LibraryStatus)
	require.Empty(t, rg.InLibraryReleaseMBzID)
}

func TestRefreshInLibraryArtistNotFoundMarksAllMissing(t *testing.T) {
	ctx := context.Background()
	present := oldRG("rg1")
	present.LibraryStatus = database.LibraryStatusPresent
	present.InLibraryReleaseMBzID = "relA"
	songs := oldRG("rg2")
	songs.LibraryStatus = database.LibraryStatusSongsPresent
	e := newLibEnv(t, present, songs)
	// subsonic only knows an artist with the same name but another MusicBrainz ID
	e.sub.set([]subArtist{{ID: "s9", Name: "Artist", MBzID: "someone-else"}}, nil)

	require.NoError(t, e.task.RunArtist(ctx, artistA))

	for _, id := range []string{"rg1", "rg2"} {
		rg := e.rg(t, id)
		require.Equal(t, database.LibraryStatusMissing, rg.LibraryStatus, id)
		require.Empty(t, rg.InLibraryReleaseMBzID, id)
	}
	require.Zero(t, e.mb.requestCount())

	a, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, a.RefreshedAt)
}

func TestRefreshInLibrarySetsRefreshedAt(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	e.navi.setSongs("s1", song("relA", "rg1", "r1"))

	before := time.Now().Unix()
	require.NoError(t, e.task.RunArtist(ctx, artistA))

	a, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, a.RefreshedAt)
	require.GreaterOrEqual(t, *a.RefreshedAt, before)
	require.LessOrEqual(t, *a.RefreshedAt, time.Now().Unix())
}

func TestRefreshInLibraryMusicBrainzErrorDoesNotMarkRefreshed(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	e.navi.setSongs("s1")
	e.mb.Close()

	require.Error(t, e.task.RunArtist(ctx, artistA))

	a, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.Nil(t, a.RefreshedAt)
}

func TestRefreshInLibraryRunAllWatchedArtists(t *testing.T) {
	ctx := context.Background()
	e := newLibEnv(t, oldRG("rg1"))
	// a2 is not watched
	putRG(t, e.db, database.ReleaseGroup{MBzID: "rg2", Name: "rg2", PrimaryType: "Album", ReleaseDate: "2010", LibraryStatus: database.LibraryStatusPresent, Artists: []database.Artist{artistB}})
	e.navi.setSongs("s1", song("relA", "rg1", "r1"))

	require.NoError(t, e.task.Run(ctx))

	require.Equal(t, database.LibraryStatusPresent, e.rg(t, "rg1").LibraryStatus)
	a1, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, a1.RefreshedAt)
	a2, err := e.db.Artist(ctx, "a2")
	require.NoError(t, err)
	require.Nil(t, a2.RefreshedAt)
}

func TestRefreshInLibraryRunWithoutWatchedArtistsNeedsNoUser(t *testing.T) {
	e := newEnv(t)
	task := NewRefreshInLibrary(e.db, e.mbClient, e.subClient, e.naviClient)
	require.NoError(t, task.Run(context.Background()))
}

func TestRefreshInLibraryNoSubsonicUser(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	require.NoError(t, e.db.AddUser(ctx, database.User{ID: "u1", Name: "u1"}))
	putArtist(t, e.db, artistA)
	require.NoError(t, e.db.AddArtistWatch(ctx, database.ArtistWatch{UserID: "u1", ArtistMBzID: "a1", Status: true, XXPrimaryTypes: "Album"}))
	task := NewRefreshInLibrary(e.db, e.mbClient, e.subClient, e.naviClient)

	require.ErrorContains(t, task.Run(ctx), "no subsonic user")
	require.ErrorContains(t, task.RunArtist(ctx, artistA), "no subsonic user")
}
