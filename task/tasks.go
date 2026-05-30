package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/musicbrainz"
	"github.com/shimmerglass/musikat/notification"
	"github.com/shimmerglass/musikat/subsonic"
)

type task interface {
	Run(context.Context) error
}

type Tasks struct {
	starred  task
	releases task
	inLib    task
	notif    task
}

func New(
	db *database.DB,
	mbz *musicbrainz.MusicBrainz,
	sub *subsonic.Subsonic,
	notifier notification.Notifier,
) *Tasks {
	return &Tasks{
		starred:  NewRefreshStarred(db, sub),
		releases: NewRefreshArtistReleases(db, mbz),
		inLib:    NewRefreshInLibrary(db, mbz, sub),
		notif:    NewSendNotifications(db, notifier),
	}
}

func (t *Tasks) Start() {
	go func() {
		tick := time.Tick(24 * time.Hour)
		for ; ; <-tick {
			err := t.Run(context.Background())
			if err != nil {
				slog.Error(err.Error())
			}
		}
	}()
}

func (t *Tasks) Run(ctx context.Context) error {
	slog.Info("refresh started")

	err := t.starred.Run(ctx)
	if err != nil {
		return err
	}

	err = t.releases.Run(ctx)
	if err != nil {
		return err
	}

	err = t.inLib.Run(ctx)
	if err != nil {
		return err
	}

	err = t.notif.Run(ctx)
	if err != nil {
		return err
	}

	slog.Info("refresh completed")

	return nil
}
