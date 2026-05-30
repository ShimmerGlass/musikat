package task

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/samber/lo"
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

	sub, err := t.subsonic(ctx)
	if err != nil {
		return err
	}

	watchedArtists, err := t.db.ListWatchedArtists(ctx)
	if err != nil {
		return err
	}

	for _, artist := range watchedArtists {
		err := t.runArtist(ctx, sub, artist)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *RefreshInLibrary) runArtist(ctx context.Context, sub *subsonic.User, artist database.Artist) error {
	slog.Info("refreshing artist in-library releases", "artist", artist.Name)

	rgs, err := t.db.ArtistReleaseGroups(ctx, artist)
	if err != nil {
		return err
	}

	inLibReleases, err := sub.ArtistReleases(ctx, artist)
	if err != nil {
		return err
	}

	for _, releaseGroup := range rgs {
		slog.Info("refreshing artist release group in-library releases", "artist", artist.Name, "release_group", releaseGroup.Name)

		allReleaseIDs, err := t.mbz.ReleaseGroupsReleases(ctx, releaseGroup.MBzID)
		if err != nil {
			return err
		}

		inLib := len(lo.Intersect(inLibReleases, allReleaseIDs)) > 0
		releaseGroup.InLibrary = inLib

		err = t.db.AddReleaseGroup(ctx, releaseGroup)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *RefreshInLibrary) subsonic(ctx context.Context) (*subsonic.User, error) {
	users, err := t.db.ListUsers(ctx)
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
