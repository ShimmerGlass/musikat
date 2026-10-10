package task

import (
	"testing"
	"time"

	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
)

func TestReleasesStale(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *int64 { v := now.Add(-d).Unix(); return &v }
	day := 24 * time.Hour

	tests := []struct {
		name  string
		fetch *int64
		date  string
		want  bool
	}{
		{"never fetched", nil, "2000-01-01", true},
		{"old release fresh cache", ago(2 * day), "2000-01-01", false},
		{"old release cache older than 30d", ago(31 * day), "2000-01-01", true},
		{"recent release cache 2h", ago(2 * time.Hour), "2026-09-01", false},
		{"recent release cache 25h", ago(25 * time.Hour), "2026-09-01", true},
		{"future release cache 2d", ago(2 * day), "2027-01-01", true},
		{"future release cache 2h", ago(2 * time.Hour), "2027-01-01", false},
		{"release 100d ago cache 5d", ago(5 * day), "2026-07-01", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rg := database.ReleaseGroup{ReleasesFetchedAt: tc.fetch, ReleaseDate: tc.date}
			require.Equal(t, tc.want, releasesStale(rg, now))
		})
	}
}

func set(v ...string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, s := range v {
		m[s] = struct{}{}
	}
	return m
}

func TestDirectRelease(t *testing.T) {
	rgToAlbums := map[string]map[string]struct{}{"rg": set("b", "a")}

	_, ok := directRelease(database.ReleaseGroup{MBzID: "other"}, rgToAlbums)
	require.False(t, ok)

	st, ok := directRelease(database.ReleaseGroup{MBzID: "rg"}, rgToAlbums)
	require.True(t, ok)
	require.Equal(t, libraryState{database.LibraryStatusPresent, "a"}, st)

	st, _ = directRelease(database.ReleaseGroup{MBzID: "rg", InLibraryReleaseMBzID: "b"}, rgToAlbums)
	require.Equal(t, libraryState{database.LibraryStatusPresent, "b"}, st)

	st, _ = directRelease(database.ReleaseGroup{MBzID: "rg", InLibraryReleaseMBzID: "zzz"}, rgToAlbums)
	require.Equal(t, "a", st.ReleaseID)
}

func TestCachedState(t *testing.T) {
	rels := []database.CachedRelease{
		{MBzID: "r2", RecordingIDs: []string{"x", "y"}},
		{MBzID: "r1", RecordingIDs: []string{"x"}},
	}

	tests := []struct {
		name     string
		albums   map[string]struct{}
		recs     []string
		releases []database.CachedRelease
		want     libraryState
	}{
		{"album match smallest", set("r2", "r1"), []string{"x"}, rels, libraryState{database.LibraryStatusPresent, "r1"}},
		{"songs present", set(), []string{"x", "y"}, rels, libraryState{Status: database.LibraryStatusSongsPresent}},
		{"missing", set(), []string{"z"}, rels, libraryState{Status: database.LibraryStatusMissing}},
		{"release without recordings", set(), []string{"x"}, []database.CachedRelease{{MBzID: "e"}}, libraryState{Status: database.LibraryStatusMissing}},
		{"no releases", set(), []string{"x"}, nil, libraryState{Status: database.LibraryStatusMissing}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, cachedState(tc.albums, tc.recs, tc.releases))
		})
	}
}
