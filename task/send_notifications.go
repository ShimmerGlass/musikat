package task

import (
	"context"
	"fmt"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/notification"
)

type SendNotifications struct {
	db       *database.DB
	notifier notification.Notifier
}

func NewSendNotifications(
	db *database.DB,
	notifier notification.Notifier,
) *SendNotifications {
	return &SendNotifications{
		db:       db,
		notifier: notifier,
	}
}

func (t *SendNotifications) Run(ctx context.Context) error {
	users, err := t.db.Users(ctx)
	if err != nil {
		return err
	}

	for _, user := range users {
		err := t.runUser(ctx, user)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *SendNotifications) runUser(ctx context.Context, user database.User) error {
	artists, err := t.db.UserWatchedArtists(ctx, user)
	if err != nil {
		return err
	}

	for _, artist := range artists {
		rgs, err := t.db.ArtistReleaseGroups(ctx, artist)
		if err != nil {
			return err
		}

		for _, rg := range rgs {
			if rg.InLibrary {
				continue
			}

			ok, err := t.db.HasReleaseGroupNotification(ctx, rg.MBzID, user.ID)
			if err != nil {
				return err
			}

			if ok {
				continue
			}

			message := fmt.Sprintf("New release from %s!\n%s\n%s", artist.Name, rg.Name, rg.MBzURL())
			err = t.notifier.Send(ctx, user, message)
			if err != nil {
				return err
			}

			err = t.db.AddReleaseGroupNotification(ctx, rg.MBzID, user.ID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
