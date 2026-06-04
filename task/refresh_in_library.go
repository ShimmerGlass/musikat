package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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

	for _, artist := range watchedArtists {
		err := t.RunArtist(ctx, artist)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *RefreshInLibrary) RunArtist(ctx context.Context, artist database.Artist) error {
	slog.Info("refreshing artist in-library releases", "artist", artist.Name)

	sub, navi, err := t.subsonic(ctx)
	if err != nil {
		return err
	}

	rgs, err := t.db.ArtistReleaseGroups(ctx, artist)
	if err != nil {
		return err
	}

	artistID, err := sub.ArtistSubsonicID(ctx, artist)
	if errors.Is(err, subsonic.ErrArtistNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	inLibReleases, err := sub.ArtistReleases(ctx, artistID)
	if err != nil {
		return err
	}

	artistSongs, err := navi.ArtistSongs(ctx, artistID)
	if err != nil {
		return err
	}

	artistRecordingIDs := lo.Map(artistSongs, func(song model.MediaFile, _ int) string { return song.MbzRecordingID })

	for _, releaseGroup := range rgs {
		if releaseGroup.InLibraryReleaseMBzID != "" && slices.Contains(inLibReleases, releaseGroup.InLibraryReleaseMBzID) {
			continue
		}

		slog.Info("refreshing artist release group in-library releases", "artist", artist.Name, "release_group", releaseGroup.Name)

		releases, err := t.mbz.ReleaseGroupsReleases(ctx, releaseGroup.MBzID)
		if err != nil {
			return err
		}
		releaseIDs := lo.Map(releases, func(rel mbz.Release, _ int) string { return string(rel.ID) })

		inLib := lo.Intersect(inLibReleases, releaseIDs)
		hasAllSongs := false

		if len(inLib) == 0 {
			for _, rel := range releases {
				relRecordingIDs := lo.FlatMap(rel.Media, func(media mbz.Medium, _ int) []string {
					return lo.Map(media.Tracks, func(track mbz.Track, _ int) string {
						return string(track.Recording.ID)
					})
				})

				if lo.Every(artistRecordingIDs, relRecordingIDs) {
					hasAllSongs = true
					break
				}
			}
		}

		err = t.db.PutReleaseGroup(ctx, releaseGroup.MBzID, func(o opt.Option[database.ReleaseGroup]) database.ReleaseGroup {
			rg := o.TakeOr(releaseGroup)
			if len(inLib) > 0 {
				rg.LibraryStatus = database.LibraryStatusPresent
				rg.InLibraryReleaseMBzID = inLib[0]
			} else if hasAllSongs {
				rg.LibraryStatus = database.LibraryStatusSongsPresent
			} else {
				rg.LibraryStatus = database.LibraryStatusMissing
			}
			return rg
		})
		if err != nil {
			return err
		}
	}

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
