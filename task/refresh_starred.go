package task

import (
	"context"
	"log/slog"

	"github.com/samber/lo"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/subsonic"
)

type RefreshStarred struct {
	db  *database.DB
	sub *subsonic.Subsonic
}

func NewRefreshStarred(db *database.DB, sub *subsonic.Subsonic) *RefreshStarred {
	return &RefreshStarred{
		db:  db,
		sub: sub,
	}
}

func (t *RefreshStarred) Run(ctx context.Context) error {
	slog.Info("refreshing starred artists")

	users, err := t.db.Users(ctx)
	if err != nil {
		return err
	}

	for _, user := range users {
		if user.SubsonicPass == "" && user.SubsonicUser == "" {
			continue
		}

		err := t.runUser(ctx, user)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *RefreshStarred) runUser(ctx context.Context, user database.User) error {
	slog.Info("refreshing user starred artists", "user", user.Name)

	sc, err := t.sub.User(user)
	if err != nil {
		return err
	}

	starred, err := sc.Starred(ctx)
	if err != nil {
		return err
	}

	existing, err := t.db.UserWatchedArtists(ctx, user)
	if err != nil {
		return err
	}
	existingIDs := lo.SliceToMap(existing, func(artist database.Artist) (string, bool) {
		return artist.MBzID, true
	})

	for _, starred := range starred {
		if existingIDs[starred.MBzID] {
			continue
		}

		err := t.db.AddArtist(ctx, starred)
		if err != nil {
			return err
		}

		_, ok, err := t.db.ArtistWatch(ctx, user.ID, starred.MBzID)
		if err != nil {
			return err
		}
		if ok {
			continue
		}

		err = t.db.AddArtistWatch(ctx, database.ArtistWatch{
			UserID:      user.ID,
			ArtistMBzID: starred.MBzID,
			Source:      "subsonic",
			Status:      true,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
