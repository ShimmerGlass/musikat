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

type batchArtistTask interface {
	ForBatch(context.Context) (artistTask, error)
}

type Tasks struct {
	cfg Config

	// tasks is the full pipeline, run on a user refresh.
	tasks []task

	// librarySync runs at boot and every Interval: Subsonic only, no
	// MusicBrainz.
	librarySync []task

	// The MusicBrainz work, spread over Interval in batches of due artists.
	db        *database.DB
	releases  artistTask
	inLibrary batchArtistTask
	notify    task

	// now is the clock, time.Now when nil.
	now func() time.Time

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
	refreshArtists := NewRefreshArtists(db, sub)
	refreshStarred := NewRefreshStarred(db, sub)
	releases := NewRefreshArtistReleases(db, mbz)
	inLibrary := NewRefreshInLibrary(db, mbz, sub, navi)
	notify := NewSendNotifications(db, notifier, mbz)

	return &Tasks{
		cfg:           cfg,
		refresh:       make(chan struct{}),
		refreshArtist: make(chan database.Artist, refreshArtistBuffer),
		tasks:         []task{refreshArtists, refreshStarred, releases, inLibrary, notify},
		librarySync:   []task{refreshArtists, refreshStarred, notify},
		db:            db,
		releases:      releases,
		inLibrary:     inLibrary,
		notify:        notify,
	}
}

func (t *Tasks) Start() {
	t.StartContext(context.Background())
}

func (t *Tasks) clock() time.Time {
	if t.now != nil {
		return t.now()
	}

	return time.Now()
}

// batchSize is the number of artists to check per checkInterval tick: the
// steady rate so that all watched artists are checked once per interval, plus
// a share of the late artists (overdue by more than one tick) so that a
// backlog is drained within one interval.
func batchSize(watched, late int, checkInterval, interval time.Duration) int {
	if watched <= 0 {
		return 0
	}
	if interval <= 0 || checkInterval >= interval {
		return watched
	}
	if checkInterval <= 0 {
		return 1
	}

	steady := (int64(watched)*int64(checkInterval) + int64(interval) - 1) / int64(interval)
	ticks := (int64(interval) + int64(checkInterval) - 1) / int64(checkInterval)
	backlog := (int64(max(late, 0)) + ticks - 1) / ticks

	return int(min(max(steady+backlog, 1), int64(watched)))
}

// StartContext starts the refresh loop, which stops when ctx is cancelled.
func (t *Tasks) StartContext(ctx context.Context) {
	go func() {
		var syncTick, checkTick <-chan time.Time
		var start chan struct{}

		if t.cfg.Enabled {
			syncTicker := time.NewTicker(t.cfg.Interval)
			defer syncTicker.Stop()
			syncTick = syncTicker.C
			start = make(chan struct{}, 1)
			start <- struct{}{}

			checkTicker := time.NewTicker(t.cfg.checkInterval())
			defer checkTicker.Stop()
			checkTick = checkTicker.C
		}

		for {
			var err error

			select {
			case <-ctx.Done():
				return
			case <-syncTick:
				err = t.syncLibrary(ctx)
			case <-start:
				err = t.syncLibrary(ctx)
			case <-checkTick:
				err = t.checkDue(ctx, t.clock())
			case <-t.refresh:
				err = t.refreshAll(ctx)
			case artist := <-t.refreshArtist:
				err = t.refreshOne(ctx, artist)
			}

			if err != nil {
				slog.Error(err.Error())
			}
		}
	}()
}

// syncLibrary syncs artists and starred artists from Subsonic and sends
// notifications. It does not use MusicBrainz.
func (t *Tasks) syncLibrary(ctx context.Context) error {
	t.running.Add(1)
	defer t.running.Add(-1)

	slog.Info("library sync started")

	for _, task := range t.librarySync {
		name := t.taskName(task)

		slog.Info("task started", "task", name)

		err := task.Run(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}

		slog.Info("task finished", "task", name)
	}

	slog.Info("library sync completed")

	return nil
}

// checkDue checks against MusicBrainz the watched artists most overdue, as many
// as needed to check every artist once per Interval. Every attempted artist
// is marked as checked, even on failure, so that a failing artist or a
// MusicBrainz outage is not retried at every tick.
func (t *Tasks) checkDue(ctx context.Context, now time.Time) error {
	t.running.Add(1)
	defer t.running.Add(-1)

	if t.db == nil {
		return nil
	}

	watched, err := t.db.WatchedArtists(ctx)
	if err != nil {
		return err
	}

	if len(watched) == 0 {
		return nil
	}

	checkInterval := t.cfg.checkInterval()

	late, err := t.db.CountDueArtists(ctx, now.Add(-t.cfg.Interval-checkInterval))
	if err != nil {
		return err
	}

	batch := batchSize(len(watched), late, checkInterval, t.cfg.Interval)

	artists, err := t.db.DueArtists(ctx, now.Add(-t.cfg.Interval), batch)
	if err != nil {
		return err
	}
	if len(artists) == 0 {
		return nil
	}

	slog.Info("checking due artists", "count", len(artists), "watched", len(watched))

	// Without Subsonic or Navidrome only the in-library step is skipped.
	inLibrary, err := t.inLibrary.ForBatch(ctx)
	if err != nil {
		slog.Warn("in-library refresh unavailable, skipping it for this batch", "error", err)
		inLibrary = nil
	}

	ids := make([]string, 0, len(artists))
	for _, artist := range artists {
		if ctx.Err() != nil {
			break
		}

		err := t.releases.RunArtist(ctx, artist)
		if err == nil && inLibrary != nil {
			err = inLibrary.RunArtist(ctx, artist)
		}
		if ctx.Err() != nil {
			// interrupted, not a real attempt
			break
		}
		if err != nil {
			slog.Error("artist check failed", "artist", artist.Name, "error", err)
		}

		ids = append(ids, artist.MBzID)
	}

	err = t.db.MarkArtistsChecked(context.WithoutCancel(ctx), ids, now)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return ctx.Err()
	}

	err = t.notify.Run(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", t.taskName(t.notify), err)
	}

	return nil
}

// refreshAll is the user-triggered refresh: it runs the whole pipeline and
// marks every watched artist as checked.
func (t *Tasks) refreshAll(ctx context.Context) error {
	err := t.Run(ctx)
	if err != nil {
		return err
	}

	if t.db == nil {
		return nil
	}

	watched, err := t.db.WatchedArtists(ctx)
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(watched))
	for _, a := range watched {
		ids = append(ids, a.MBzID)
	}

	return t.db.MarkArtistsChecked(ctx, ids, t.clock())
}

func (t *Tasks) refreshOne(ctx context.Context, artist database.Artist) error {
	err := t.RunArtist(ctx, artist)
	if err != nil {
		return err
	}

	if t.db == nil {
		return nil
	}

	return t.db.MarkArtistsChecked(ctx, []string{artist.MBzID}, t.clock())
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

// QueuedArtists is the number of artist refreshes waiting to be run.
func (t *Tasks) QueuedArtists() int {
	return len(t.refreshArtist)
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
