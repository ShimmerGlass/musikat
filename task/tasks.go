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
	"github.com/shimmerglass/musikat/navidrome"
	"github.com/shimmerglass/musikat/notification"
	"github.com/shimmerglass/musikat/subsonic"
)

const refreshArtistBuffer = 32

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
	running       atomic.Int32
}

func New(
	cfg Config,
	db *database.DB,
	mbz *musicbrainz.MusicBrainz,
	sub *subsonic.Subsonic,
	navi *navidrome.Client,
	notifier notification.Notifier,
) *Tasks {
	return &Tasks{
		cfg:           cfg,
		refresh:       make(chan struct{}),
		refreshArtist: make(chan database.Artist, refreshArtistBuffer),
		tasks: []task{
			NewRefreshArtists(db, sub),
			NewRefreshStarred(db, sub),
			NewRefreshArtistReleases(db, mbz),
			NewRefreshInLibrary(db, mbz, sub, navi),
			NewSendNotifications(db, notifier, mbz),
		},
	}
}

func (t *Tasks) Start() {
	t.StartContext(context.Background())
}

// StartContext starts the refresh loop, which stops when ctx is cancelled.
func (t *Tasks) StartContext(ctx context.Context) {
	go func() {
		var tick <-chan time.Time
		var start chan struct{}

		if t.cfg.Enabled {
			ticker := time.NewTicker(t.cfg.Interval)
			defer ticker.Stop()
			tick = ticker.C
			start = make(chan struct{}, 1)
			start <- struct{}{}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-tick:
			case <-start:
			case <-t.refresh:

			case artist := <-t.refreshArtist:
				err := t.RunArtist(ctx, artist)
				if err != nil {
					slog.Error(err.Error())
				}

				continue
			}

			err := t.Run(ctx)
			if err != nil {
				slog.Error(err.Error())
			}
		}
	}()
}

func (t *Tasks) Run(ctx context.Context) error {
	t.running.Add(1)
	defer t.running.Add(-1)

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
	t.running.Add(1)
	defer t.running.Add(-1)

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
	return t.running.Load() > 0
}

func (t *Tasks) Refresh() {
	select {
	case t.refresh <- struct{}{}:
	default:
	}
}

func (t *Tasks) RefreshArtist(artist database.Artist) {
	select {
	case t.refreshArtist <- artist:
	default:
		slog.Warn("artist refresh queue full, dropping request", "artist", artist.Name)
	}
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
