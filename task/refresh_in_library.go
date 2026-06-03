package task

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/samber/lo"
	opt "github.com/shimmerglass/go-optional"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/musicbrainz"
	"github.com/shimmerglass/musikat/subsonic"
)

type RefreshInLibrary struct {
	db  *database.DB
	mbz *musicbrainz.MusicBrainz
	sub *subsonic.Subsonic
}

func NewRefreshInLibrary(
	db *database.DB,
	mbz *musicbrainz.MusicBrainz,
	sub *subsonic.Subsonic,
) *RefreshInLibrary {
	return &RefreshInLibrary{
		db:  db,
		mbz: mbz,
		sub: sub,
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

	sub, err := t.subsonic(ctx)
	if err != nil {
		return err
	}

	rgs, err := t.db.ArtistReleaseGroups(ctx, artist)
	if err != nil {
		return err
	}

	inLibReleases, err := sub.ArtistReleases(ctx, artist)
	if err != nil {
		return err
	}

	for _, releaseGroup := range rgs {
		if releaseGroup.InLibraryReleaseMBzID != "" && slices.Contains(inLibReleases, releaseGroup.InLibraryReleaseMBzID) {
			continue
		}

		slog.Info("refreshing artist release group in-library releases", "artist", artist.Name, "release_group", releaseGroup.Name)

		allReleaseIDs, err := t.mbz.ReleaseGroupsReleases(ctx, releaseGroup.MBzID)
		if err != nil {
			return err
		}

		inLib := lo.Intersect(inLibReleases, allReleaseIDs)
		err = t.db.PutReleaseGroup(ctx, releaseGroup.MBzID, func(o opt.Option[database.ReleaseGroup]) database.ReleaseGroup {
			rg := o.TakeOr(releaseGroup)
			if len(inLib) > 0 {
				rg.LibraryStatus = database.LibraryStatusPresent
				rg.InLibraryReleaseMBzID = inLib[0]
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

func (t *RefreshInLibrary) subsonic(ctx context.Context) (*subsonic.User, error) {
	users, err := t.db.Users(ctx)
	if err != nil {
		return nil, err
	}

	for _, user := range users {
		if user.SubsonicUser != "" && user.SubsonicPass != "" {
			return t.sub.User(user)
		}
	}

	return nil, fmt.Errorf("no subsonic user found")
}
