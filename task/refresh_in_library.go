package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/samber/lo"
	opt "github.com/shimmerglass/go-optional"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/musicbrainz"
	"github.com/shimmerglass/musikat/navidrome"
	"github.com/shimmerglass/musikat/subsonic"
	mbz "go.uploadedlobster.com/musicbrainzws2"
)

type RefreshInLibrary struct {
	db   *database.DB
	mbz  *musicbrainz.MusicBrainz
	sub  *subsonic.Subsonic
	navi *navidrome.Client
}

func NewRefreshInLibrary(
	db *database.DB,
	mbz *musicbrainz.MusicBrainz,
	sub *subsonic.Subsonic,
	navi *navidrome.Client,
) *RefreshInLibrary {
	return &RefreshInLibrary{
		db:   db,
		mbz:  mbz,
		sub:  sub,
		navi: navi,
	}
}

func (t *RefreshInLibrary) Run(ctx context.Context) error {
	slog.Info("refreshing in-library releases")

	watchedArtists, err := t.db.WatchedArtists(ctx)
	if err != nil {
		return err
	}

	if len(watchedArtists) == 0 {
		return nil
	}

	sub, navi, err := t.subsonic(ctx)
	if err != nil {
		return err
	}

	for _, artist := range watchedArtists {
		err := t.runArtist(ctx, artist, sub, navi)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *RefreshInLibrary) RunArtist(ctx context.Context, artist database.Artist) error {
	sub, navi, err := t.subsonic(ctx)
	if err != nil {
		return err
	}

	return t.runArtist(ctx, artist, sub, navi)
}

func (t *RefreshInLibrary) runArtist(ctx context.Context, artist database.Artist, sub *subsonic.User, navi *navidrome.User) error {
	slog.Info("refreshing artist in-library releases", "artist", artist.Name)

	rgs, err := t.db.ArtistReleaseGroups(ctx, artist)
	if err != nil {
		return err
	}

	artistID, err := sub.ArtistSubsonicID(ctx, artist)
	if errors.Is(err, subsonic.ErrArtistNotFound) {
		err = t.markAllMissing(ctx, rgs)
		if err != nil {
			return err
		}

		return t.markRefreshed(ctx, artist)
	}
	if err != nil {
		return err
	}

	artistSongs, err := navi.ArtistSongs(ctx, artistID)
	if err != nil {
		return err
	}

	artistRecordingIDs := lo.Map(artistSongs, func(song model.MediaFile, _ int) string { return song.MbzRecordingID })
	rgToAlbums := map[string]map[string]struct{}{}
	songAlbumIDs := map[string]struct{}{}
	for _, song := range artistSongs {
		if song.MbzAlbumID == "" {
			continue
		}
		songAlbumIDs[song.MbzAlbumID] = struct{}{}
		if song.MbzReleaseGroupID == "" {
			continue
		}
		if rgToAlbums[song.MbzReleaseGroupID] == nil {
			rgToAlbums[song.MbzReleaseGroupID] = map[string]struct{}{}
		}
		rgToAlbums[song.MbzReleaseGroupID][song.MbzAlbumID] = struct{}{}
	}

	for _, releaseGroup := range rgs {
		state, ok := directRelease(releaseGroup, rgToAlbums)
		if !ok {
			cached, err := t.db.ReleaseGroupReleases(ctx, releaseGroup.MBzID)
			if err != nil {
				return err
			}

			if releasesStale(releaseGroup, time.Now()) {
				slog.Info("fetching release group releases", "artist", artist.Name, "release_group", releaseGroup.Name)

				releases, err := t.mbz.ReleaseGroupsReleases(ctx, releaseGroup.MBzID)
				if err != nil {
					return err
				}

				cached = lo.Map(releases, func(rel mbz.Release, _ int) database.CachedRelease {
					return database.CachedRelease{
						MBzID: string(rel.ID),
						RecordingIDs: lo.FlatMap(rel.Media, func(media mbz.Medium, _ int) []string {
							return lo.Map(media.Tracks, func(track mbz.Track, _ int) string {
								return string(track.Recording.ID)
							})
						}),
					}
				})

				err = t.db.ReplaceReleaseGroupReleases(ctx, releaseGroup.MBzID, cached, time.Now())
				if err != nil {
					return err
				}
			}

			state = cachedState(songAlbumIDs, artistRecordingIDs, cached)
		}

		err = t.setLibraryState(ctx, releaseGroup, state)
		if err != nil {
			return err
		}
	}

	return t.markRefreshed(ctx, artist)
}

// markAllMissing handles an artist with no library content at all.
func (t *RefreshInLibrary) markAllMissing(ctx context.Context, rgs []database.ReleaseGroup) error {
	for _, rg := range rgs {
		err := t.setLibraryState(ctx, rg, libraryState{Status: database.LibraryStatusMissing})
		if err != nil {
			return err
		}
	}

	return nil
}

// setLibraryState writes the library state of a release group, only if it
// differs from the current one.
func (t *RefreshInLibrary) setLibraryState(ctx context.Context, releaseGroup database.ReleaseGroup, state libraryState) error {
	if state.Status == releaseGroup.LibraryStatus && state.ReleaseID == releaseGroup.InLibraryReleaseMBzID {
		return nil
	}

	return t.db.PutReleaseGroup(ctx, releaseGroup.MBzID, func(o opt.Option[database.ReleaseGroup]) database.ReleaseGroup {
		rg := o.TakeOr(releaseGroup)
		rg.LibraryStatus = state.Status
		rg.InLibraryReleaseMBzID = state.ReleaseID
		return rg
	})
}

func (t *RefreshInLibrary) markRefreshed(ctx context.Context, artist database.Artist) error {
	return t.db.PutArtist(ctx, artist.MBzID, func(o opt.Option[database.Artist]) database.Artist {
		a := o.TakeOr(artist)
		a.RefreshedAt = new(time.Now().Unix())
		return a
	})
}

func (t *RefreshInLibrary) subsonic(ctx context.Context) (*subsonic.User, *navidrome.User, error) {
	users, err := t.db.Users(ctx)
	if err != nil {
		return nil, nil, err
	}

	for _, user := range users {
		if user.SubsonicUser != "" && user.SubsonicPass != "" {
			sub, err := t.sub.User(user)
			if err != nil {
				return nil, nil, err
			}

			navi, err := t.navi.User(ctx, user.SubsonicUser, user.SubsonicPass)
			if err != nil {
				return nil, nil, err
			}

			return sub, navi, nil

		}
	}

	return nil, nil, fmt.Errorf("no subsonic user found")
}
