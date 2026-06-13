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

	for _, artist := range artists {
		err := t.RunArtist(ctx, artist)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *RefreshArtistReleases) RunArtist(ctx context.Context, artist database.Artist) error {
	slog.Info("refreshing artist releases", "artist", artist.Name)

	existingRGs, err := t.db.ArtistReleaseGroups(ctx, artist)
	if err != nil {
		return err
	}

	newRGs, err := t.mbz.ArtistReleaseGroups(ctx, artist.MBzID)
	if err != nil {
		return err
	}

	for _, rg := range newRGs {
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

	newIDs := lo.SliceToMap(newRGs, func(rg database.ReleaseGroup) (string, bool) {
		return rg.MBzID, true
	})

	for _, existingRG := range existingRGs {
		if newIDs[existingRG.MBzID] {
			continue
		}

		slog.Info("deleting release group removed from musicbrainz", "name", existingRG.Name, "id", existingRG.MBzID)
		t.db.DeleteReleaseGroup(ctx, existingRG.MBzID)
	}

	return nil
}
