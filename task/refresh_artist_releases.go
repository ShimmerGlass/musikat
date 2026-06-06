package task

import (
	"context"
	"log/slog"

	"github.com/samber/lo"
	opt "github.com/shimmerglass/go-optional"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/musicbrainz"
)

type RefreshArtistReleases struct {
	db  *database.DB
	mbz *musicbrainz.MusicBrainz
}

func NewRefreshArtistReleases(db *database.DB, mbz *musicbrainz.MusicBrainz) *RefreshArtistReleases {
	return &RefreshArtistReleases{
		db:  db,
		mbz: mbz,
	}
}

func (t *RefreshArtistReleases) Run(ctx context.Context) error {
	slog.Info("refreshing artist releases")

	artists, err := t.db.WatchedArtists(ctx)
	if err != nil {
		return err
	}

	existing, err := t.db.ReleaseGroups(ctx)
	if err != nil {
		return err
	}
	existingMap := lo.SliceToMap(existing, func(rg database.ReleaseGroup) (string, database.ReleaseGroup) {
		return rg.MBzID, rg
	})

	for _, artist := range artists {
		err := t.runArtist(ctx, artist, existingMap)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *RefreshArtistReleases) RunArtist(ctx context.Context, artist database.Artist) error {
	existing, err := t.db.ReleaseGroups(ctx)
	if err != nil {
		return err
	}
	existingMap := lo.SliceToMap(existing, func(rg database.ReleaseGroup) (string, database.ReleaseGroup) {
		return rg.MBzID, rg
	})

	return t.runArtist(ctx, artist, existingMap)
}

func (t *RefreshArtistReleases) runArtist(ctx context.Context, artist database.Artist, existingMap map[string]database.ReleaseGroup) error {
	slog.Info("refreshing artist releases", "artist", artist.Name)

	rgs, err := t.mbz.ArtistReleaseGroups(ctx, artist.MBzID)
	if err != nil {
		return err
	}
	rgs = lo.Map(rgs, func(rg database.ReleaseGroup, _ int) database.ReleaseGroup {
		rg.LibraryStatus = existingMap[rg.MBzID].LibraryStatus
		return rg
	})

	for _, rg := range rgs {
		for _, rgArtist := range rg.Artists {
			err := t.db.PutArtist(ctx, rgArtist.MBzID, func(o opt.Option[database.Artist]) database.Artist {
				return o.TakeOr(rgArtist)
			})
			if err != nil {
				return err
			}
		}

		err := t.db.PutReleaseGroup(ctx, rg.MBzID, func(o opt.Option[database.ReleaseGroup]) database.ReleaseGroup {
			r := o.TakeOr(rg)
			r.Name = rg.Name
			r.PrimaryType = rg.PrimaryType
			r.XXSecondaryTypes = rg.XXSecondaryTypes
			r.ReleaseDate = rg.ReleaseDate

			return r
		})
		if err != nil {
			return err
		}

		err = t.db.ReplaceReleaseGroupArtists(ctx, rg)
		if err != nil {
			return err
		}
	}

	return nil
}
