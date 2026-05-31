package task

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
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

type artistTask interface {
	RunArtist(context.Context, database.Artist) error
}

type Tasks struct {
	cfg Config

	tasks []task

	refresh       chan struct{}
	refreshArtist chan database.Artist
	running       atomic.Bool
}

func New(
	cfg Config,
	db *database.DB,
	mbz *musicbrainz.MusicBrainz,
	sub *subsonic.Subsonic,
	notifier notification.Notifier,
) *Tasks {
	return &Tasks{
		cfg:           cfg,
		refresh:       make(chan struct{}),
		refreshArtist: make(chan database.Artist),
		tasks: []task{
			NewRefreshStarred(db, sub),
			NewRefreshArtistReleases(db, mbz),
			NewRefreshInLibrary(db, mbz, sub),
			NewSendNotifications(db, notifier),
		},
	}
}

func (t *Tasks) Start() {
	go func() {
		var tick <-chan time.Time
		var start chan struct{}

		if t.cfg.Enabled {
			tick = time.Tick(t.cfg.Interval)
			start = make(chan struct{}, 1)
			start <- struct{}{}
		}

		for {
			select {
			case <-tick:
			case <-start:
			case <-t.refresh:

			case artist := <-t.refreshArtist:
				err := t.RunArtist(context.Background(), artist)
				if err != nil {
					slog.Error(err.Error())
				}

				continue
			}

			err := t.Run(context.Background())
			if err != nil {
				slog.Error(err.Error())
			}
		}
	}()
}

func (t *Tasks) Run(ctx context.Context) error {
	t.running.Store(true)
	defer t.running.Store(false)

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

func (t *Tasks) RunArtist(ctx context.Context, artist database.Artist) error {
	t.running.Store(true)
	defer t.running.Store(false)

	slog.Info("refresh started", "artist", artist.Name)

	for _, task := range t.tasks {
		name := t.taskName(task)

		slog.Info("task started", "task", name)

		if at, ok := task.(artistTask); ok {
			err := at.RunArtist(ctx, artist)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		} else {
			err := task.Run(ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}

		slog.Info("task finished", "task", name)
	}

	slog.Info("refresh completed", "artist", artist.Name)

	return nil
}

func (t *Tasks) Running() bool {
	return t.running.Load()
}

func (t *Tasks) Refresh() {
	select {
	case t.refresh <- struct{}{}:
	default:
	}
}

func (t *Tasks) RefreshArtist(artist database.Artist) {
	t.refreshArtist <- artist
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
