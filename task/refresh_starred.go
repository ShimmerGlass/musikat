package task

import (
	"context"
	"log/slog"

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

	for _, starred := range starred {
		err := t.db.AddArtist(ctx, starred)
		if err != nil {
			return err
		}

		err = t.db.AddArtistWatch(ctx, database.ArtistWatch{
			UserID:      user.ID,
			ArtistMBzID: starred.MBzID,
			Source:      "subsonic",
		})
		if err != nil {
			return err
		}
	}

	return nil
}
