package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/notification"
)

type coverArtFinder interface {
	ReleaseGroupCoverURL(ctx context.Context, rgID string) (string, error)
}

type SendNotifications struct {
	db       *database.DB
	notifier notification.Notifier
	covers   coverArtFinder
}

func NewSendNotifications(
	db *database.DB,
	notifier notification.Notifier,
	covers coverArtFinder,
) *SendNotifications {
	return &SendNotifications{
		db:       db,
		notifier: notifier,
		covers:   covers,
	}
}

func (t *SendNotifications) Run(ctx context.Context) error {
	users, err := t.db.Users(ctx)
	if err != nil {
		return err
	}

	var errs []error
	for _, user := range users {
		err := t.runUser(ctx, user)
		if err != nil {
			slog.Error("sending notifications failed", "user", user.Name, "error", err)
			errs = append(errs, fmt.Errorf("user %s: %w", user.Name, err))
		}
	}

	return errors.Join(errs...)
}

// shouldNotify reports whether a notification is due for rg, ignoring whether
// the user was already notified. A release is out once its date is reached
// (for partial dates, once the whole period has elapsed), and is ignored if
// its whole period ended before the watch was added.
func shouldNotify(rg database.ReleaseGroup, watch database.ArtistWatch, now time.Time) bool {
	if !watch.Status {
		return false
	}

	if rg.LibraryStatus != database.LibraryStatusMissing {
		return false
	}

	if !watch.Watches(rg) {
		return false
	}

	end := rg.ReleaseEnd()
	if rg.IsFullDate() {
		if rg.ReleaseTime().After(now) {
			return false
		}
	} else if end.After(now) {
		return false
	}

	return end.After(time.Unix(watch.AddedAt, 0))
}

func (t *SendNotifications) runUser(ctx context.Context, user database.User) error {
	watches, err := t.db.UserArtistWatches(ctx, user)
	if err != nil {
		return err
	}

	notified, err := t.db.NotifiedReleaseGroupIDs(ctx, user.ID)
	if err != nil {
		return err
	}

	now := time.Now()

	for _, watch := range watches {
		if !watch.Status {
			continue
		}

		artist, err := t.db.Artist(ctx, watch.ArtistMBzID)
		if err != nil {
			return err
		}

		rgs, err := t.db.ArtistReleaseGroupsWithoutArtists(ctx, artist)
		if err != nil {
			return err
		}

		for _, rg := range rgs {
			if !shouldNotify(rg, watch, now) {
				continue
			}

			if _, ok := notified[rg.MBzID]; ok {
				continue
			}

			// Record first so a crash between the two steps can never lead to
			// duplicate messages; undo the record if sending fails.
			err = t.db.AddReleaseGroupNotification(ctx, rg.MBzID, user.ID)
			if err != nil {
				return err
			}
			notified[rg.MBzID] = struct{}{}

			message := fmt.Sprintf("New release from %s!\n%s (%s)\n%s", artist.Name, rg.Name, rg.TypeLabel(), rg.MBzURL())
			coverURL, coverErr := t.covers.ReleaseGroupCoverURL(ctx, rg.MBzID)
			if coverErr != nil {
				slog.Warn("cover art lookup failed", "release_group", rg.Name, "error", coverErr)
			} else if coverURL != "" {
				message += "\n" + coverURL
			}

			err = t.notifier.Send(ctx, user, message)
			if err != nil {
				slog.Error("sending notification failed", "user", user.Name, "release_group", rg.Name, "error", err)

				delErr := t.db.DeleteReleaseGroupNotification(ctx, rg.MBzID, user.ID)
				if delErr != nil {
					slog.Error("removing notification record failed", "user", user.Name, "release_group", rg.Name, "error", delErr)
				} else {
					delete(notified, rg.MBzID)
				}
			}
		}
	}

	return nil
}
