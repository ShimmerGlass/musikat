package task

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/musicbrainz"
	"github.com/shimmerglass/musikat/notification"
	"github.com/shimmerglass/musikat/subsonic"
)

type task interface {
	Run(context.Context) error
}

type Tasks struct {
	cfg Config

	tasks []task
}

func New(
	cfg Config,
	db *database.DB,
	mbz *musicbrainz.MusicBrainz,
	sub *subsonic.Subsonic,
	notifier notification.Notifier,
) *Tasks {
	return &Tasks{
		cfg: cfg,
		tasks: []task{
			NewRefreshStarred(db, sub),
			NewRefreshArtistReleases(db, mbz),
			NewRefreshInLibrary(db, mbz, sub),
			NewSendNotifications(db, notifier),
		},
	}
}

func (t *Tasks) Start() {
	if !t.cfg.Enabled {
		return
	}

	go func() {
		tick := time.Tick(t.cfg.Interval)
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

	for _, task := range t.tasks {
		name := t.taskName(task)

		slog.Info("task started", "task", name)

		err := task.Run(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		slog.Info("task finished", "task", name)
	}

	slog.Info("refresh completed")

	return nil
}

func (t *Tasks) taskName(to task) string {
	_, st, _ := strings.Cut(fmt.Sprintf("%T", to), ".")
	return toSnakeCase(st)
}

func toSnakeCase(s string) string {
	var b strings.Builder

	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}

	return b.String()
}
