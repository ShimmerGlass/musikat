package task

import (
	"slices"
	"time"

	"github.com/samber/lo"
	"github.com/shimmerglass/musikat/database"
)

const (
	releasesMaxAge       = 30 * 24 * time.Hour
	recentReleaseWindow  = 90 * 24 * time.Hour
	recentReleasesMaxAge = 24 * time.Hour
)

// releasesStale reports whether the cached MusicBrainz releases of a release
// group should be fetched again.
func releasesStale(rg database.ReleaseGroup, now time.Time) bool {
	if rg.ReleasesFetchedAt == nil {
		return true
	}

	age := now.Sub(time.Unix(*rg.ReleasesFetchedAt, 0))
	if age > releasesMaxAge {
		return true
	}

	recent := rg.ReleaseTime().After(now.Add(-recentReleaseWindow))
	return recent && age > recentReleasesMaxAge
}

// libraryState is the in-library status of a release group.
type libraryState struct {
	Status    database.LibraryStatus
	ReleaseID string
}

// directRelease resolves a release group that the library songs reference
// directly through their release group ID. ok is false if there is none.
func directRelease(rg database.ReleaseGroup, rgToAlbums map[string]map[string]struct{}) (libraryState, bool) {
	albums := rgToAlbums[rg.MBzID]
	if len(albums) == 0 {
		return libraryState{}, false
	}

	if _, ok := albums[rg.InLibraryReleaseMBzID]; ok {
		return libraryState{database.LibraryStatusPresent, rg.InLibraryReleaseMBzID}, true
	}

	smallest := lo.Min(lo.Keys(albums))
	return libraryState{database.LibraryStatusPresent, smallest}, true
}

// cachedState resolves a release group from its cached MusicBrainz releases.
func cachedState(
	songAlbumIDs map[string]struct{},
	artistRecordingIDs []string,
	releases []database.CachedRelease,
) libraryState {
	inLib := []string{}
	for _, rel := range releases {
		if _, ok := songAlbumIDs[rel.MBzID]; ok {
			inLib = append(inLib, rel.MBzID)
		}
	}
	if len(inLib) > 0 {
		slices.Sort(inLib)
		return libraryState{database.LibraryStatusPresent, inLib[0]}
	}

	for _, rel := range releases {
		if lo.Every(artistRecordingIDs, rel.RecordingIDs) {
			return libraryState{Status: database.LibraryStatusSongsPresent}
		}
	}

	return libraryState{Status: database.LibraryStatusMissing}
}
