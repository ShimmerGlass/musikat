package task

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/subsonic"
)

type RefreshArtists struct {
	db  *database.DB
	sub *subsonic.Subsonic
}

func NewRefreshArtists(db *database.DB, sub *subsonic.Subsonic) *RefreshArtists {
	return &RefreshArtists{
		db:  db,
		sub: sub,
	}
}

func (t *RefreshArtists) Run(ctx context.Context) error {
	slog.Info("refreshing all artists")

	sub, err := t.subsonic(ctx)
	if err != nil {
		return err
	}

	artists, err := sub.Artists(ctx)
	if err != nil {
		return err
	}

	for _, artist := range artists {
		err := t.db.AddArtist(ctx, artist)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *RefreshArtists) RunArtist(ctx context.Context, artist database.Artist) error {
	return nil
}

func (t *RefreshArtists) subsonic(ctx context.Context) (*subsonic.User, error) {
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
