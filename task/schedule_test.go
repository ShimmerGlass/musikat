package task

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
)

func TestBatchSize(t *testing.T) {
	const day = 24 * time.Hour
	for name, tc := range map[string]struct {
		watched, late int
		check, interv time.Duration
		want          int
	}{
		"none watched":           {0, 0, 15 * time.Minute, day, 0},
		"exact":                  {96, 0, 15 * time.Minute, day, 1},
		"rounds up":              {97, 0, 15 * time.Minute, day, 2},
		"one artist":             {1, 0, 15 * time.Minute, day, 1},
		"many":                   {960, 0, 15 * time.Minute, day, 10},
		"check longer":           {5, 0, 2 * day, day, 5},
		"check equals interval":  {7, 0, day, day, 7},
		"zero check interval":    {7, 0, 0, day, 1},
		"zero interval":          {7, 0, time.Minute, 0, 7},
		"negative watched":       {-1, 0, time.Minute, day, 0},
		"check longer, no watch": {0, 0, 2 * day, day, 0},
		"late adds a share":      {960, 960, 15 * time.Minute, day, 20},
		"late rounds up":         {960, 1, 15 * time.Minute, day, 11},
		"late capped at watched": {10, 10, 12 * time.Hour, day, 10},
		"negative late":          {96, -5, 15 * time.Minute, day, 1},
		"late, check longer":     {5, 5, 2 * day, day, 5},
	} {
		require.Equal(t, tc.want, batchSize(tc.watched, tc.late, tc.check, tc.interv), name)
	}
}

func TestConfigCheckInterval(t *testing.T) {
	require.Equal(t, time.Hour, Config{Interval: 24 * time.Hour, CheckInterval: time.Hour}.checkInterval())
	require.Equal(t, 24*time.Hour, Config{Interval: 24 * time.Hour}.checkInterval())
	require.Equal(t, 24*time.Hour, Config{Interval: 24 * time.Hour, CheckInterval: -1}.checkInterval())
}

// schedEnv has n watched artists a1..an and a Tasks built with New.
type schedEnv struct {
	*env
	tasks *Tasks
	n     *fakeNotifier
	now   time.Time
}

func newSchedEnv(t *testing.T, n int, cfg Config) *schedEnv {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	for i := 1; i <= n; i++ {
		a := database.Artist{MBzID: fmt.Sprintf("a%d", i), Name: fmt.Sprintf("Artist %d", i)}
		putArtist(t, e.db, a)
		require.NoError(t, e.db.AddArtistWatch(ctx, database.ArtistWatch{UserID: "u1", ArtistMBzID: a.MBzID, Status: true, XXPrimaryTypes: "Album"}))
	}
	e.sub.set(nil, nil)
	notifier := &fakeNotifier{}
	s := &schedEnv{env: e, n: notifier, now: time.Now()}
	s.tasks = New(cfg, e.db, e.mbClient, e.subClient, e.naviClient, notifier)
	s.tasks.now = func() time.Time { return s.now }
	return s
}

func (s *schedEnv) checkedAt(t *testing.T, id string) *int64 {
	a, err := s.db.Artist(context.Background(), id)
	require.NoError(t, err)
	return a.CheckedAt
}

func TestCheckDueProcessesOnlyBatchOldestFirst(t *testing.T) {
	ctx := context.Background()
	// 4 artists, 6h check over 24h: steady batch of 1, plus 1 for the late
	// (never checked) a4
	s := newSchedEnv(t, 4, Config{Interval: 24 * time.Hour, CheckInterval: 6 * time.Hour})
	require.NoError(t, s.db.MarkArtistsChecked(ctx, []string{"a1"}, s.now.Add(-20*time.Hour)))
	require.NoError(t, s.db.MarkArtistsChecked(ctx, []string{"a2"}, s.now.Add(-30*time.Hour)))
	require.NoError(t, s.db.MarkArtistsChecked(ctx, []string{"a3"}, s.now.Add(-25*time.Hour)))
	// a4 never checked

	require.NoError(t, s.tasks.checkDue(ctx, s.now))

	require.Equal(t, 2, s.mb.requestCount())
	require.Equal(t, 1, s.mb.artistRequests("a4"))
	require.Equal(t, 1, s.mb.artistRequests("a2"))
	require.Equal(t, s.now.Unix(), *s.checkedAt(t, "a4"))
	require.Equal(t, s.now.Add(-25*time.Hour).Unix(), *s.checkedAt(t, "a3"))

	// nothing is late anymore: the batch is the steady one, the oldest
	require.NoError(t, s.tasks.checkDue(ctx, s.now))
	require.Equal(t, 1, s.mb.artistRequests("a3"))
	require.Equal(t, 3, s.mb.requestCount())
	require.False(t, s.tasks.Running())
}

func TestCheckDueNothingDue(t *testing.T) {
	ctx := context.Background()
	s := newSchedEnv(t, 2, Config{Interval: 24 * time.Hour, CheckInterval: time.Hour})
	require.NoError(t, s.db.MarkArtistsChecked(ctx, []string{"a1", "a2"}, s.now.Add(-time.Hour)))

	require.NoError(t, s.tasks.checkDue(ctx, s.now))

	require.Zero(t, s.mb.requestCount())
}

func TestCheckDueNoWatchedArtists(t *testing.T) {
	s := newSchedEnv(t, 0, Config{Interval: 24 * time.Hour, CheckInterval: time.Hour})
	require.NoError(t, s.tasks.checkDue(context.Background(), s.now))
	require.Zero(t, s.mb.requestCount())
}

func TestCheckDueFailingArtistIsStillMarked(t *testing.T) {
	ctx := context.Background()
	s := newSchedEnv(t, 2, Config{Interval: 24 * time.Hour, CheckInterval: 24 * time.Hour})
	s.mb.failArtists = map[string]bool{"a1": true}
	today := s.now.UTC().Format("2006-01-02")
	s.mb.rgs["a2"] = []mbRG{{ID: "rg2", Title: "Fresh", Primary: "Album", Date: today, Artists: []database.Artist{{MBzID: "a2", Name: "Artist 2"}}}}

	require.NoError(t, s.tasks.checkDue(ctx, s.now))

	require.NotNil(t, s.checkedAt(t, "a1"))
	require.NotNil(t, s.checkedAt(t, "a2"))
	_, err := s.db.ReleaseGroup(ctx, "rg2")
	require.NoError(t, err, "other artists are still processed")

	// not retried until due again
	before := s.mb.requestCount()
	require.NoError(t, s.tasks.checkDue(ctx, s.now.Add(time.Hour)))
	require.Equal(t, before, s.mb.requestCount())
}

func TestCheckDueNotifiesOncePerBatch(t *testing.T) {
	ctx := context.Background()
	s := newSchedEnv(t, 3, Config{Interval: 24 * time.Hour, CheckInterval: 24 * time.Hour})
	today := s.now.UTC().Format("2006-01-02")
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("a%d", i)
		s.mb.rgs[id] = []mbRG{{ID: "rg" + id, Title: "New " + id, Primary: "Album", Date: today,
			Artists: []database.Artist{{MBzID: id, Name: "Artist " + id[1:]}}}}
	}

	require.NoError(t, s.tasks.checkDue(ctx, s.now))

	require.Len(t, s.n.msgs, 3) // one message per release, sent by a single run
	require.NoError(t, s.tasks.checkDue(ctx, s.now.Add(time.Minute)))
	require.Len(t, s.n.msgs, 3)
}

func TestCheckDueSpreadsOverInterval(t *testing.T) {
	ctx := context.Background()
	const n = 10
	cfg := Config{Interval: 24 * time.Hour, CheckInterval: 3 * time.Hour}
	s := newSchedEnv(t, n, cfg)

	ticks := int(cfg.Interval / cfg.CheckInterval)
	for range ticks {
		require.NoError(t, s.tasks.checkDue(ctx, s.now))
		s.now = s.now.Add(cfg.CheckInterval)
	}

	for i := 1; i <= n; i++ {
		require.Equal(t, 1, s.mb.artistRequests(fmt.Sprintf("a%d", i)), "a%d", i)
	}
	require.Equal(t, n, s.mb.requestCount())

	// a second interval checks everyone once more
	for range ticks {
		require.NoError(t, s.tasks.checkDue(ctx, s.now))
		s.now = s.now.Add(cfg.CheckInterval)
	}
	require.Equal(t, 2*n, s.mb.requestCount())
}

func TestRefreshAllChecksEverythingAndMarksChecked(t *testing.T) {
	ctx := context.Background()
	s := newSchedEnv(t, 5, Config{Interval: 24 * time.Hour, CheckInterval: time.Hour})

	require.NoError(t, s.tasks.refreshAll(ctx))

	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("a%d", i)
		require.Equal(t, 1, s.mb.artistRequests(id), id)
		require.Equal(t, s.now.Unix(), *s.checkedAt(t, id))
	}

	// the spread does not recheck them right away
	before := s.mb.requestCount()
	require.NoError(t, s.tasks.checkDue(ctx, s.now.Add(time.Hour)))
	require.Equal(t, before, s.mb.requestCount())
}

func TestRefreshOneMarksArtistChecked(t *testing.T) {
	ctx := context.Background()
	s := newSchedEnv(t, 2, Config{Interval: 24 * time.Hour, CheckInterval: time.Hour})

	require.NoError(t, s.tasks.refreshOne(ctx, database.Artist{MBzID: "a1", Name: "Artist 1"}))

	require.Equal(t, s.now.Unix(), *s.checkedAt(t, "a1"))
	require.Nil(t, s.checkedAt(t, "a2"))
}

func TestSyncLibraryMakesNoMusicBrainzRequest(t *testing.T) {
	ctx := context.Background()
	s := newSchedEnv(t, 0, Config{Interval: 24 * time.Hour, CheckInterval: time.Hour})
	s.sub.set(
		[]subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}},
		[]subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}},
	)

	require.NoError(t, s.tasks.syncLibrary(ctx))

	require.Zero(t, s.mb.requestCount())
	_, err := s.db.Artist(ctx, "a1")
	require.NoError(t, err, "subsonic artists are synced")
	w, ok, err := s.db.ArtistWatch(ctx, "u1", "a1")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, w.Status)
	require.Nil(t, s.checkedAt(t, "a1"))
	require.False(t, s.tasks.Running())
}

func TestSyncLibraryKeepsCheckedAt(t *testing.T) {
	ctx := context.Background()
	s := newSchedEnv(t, 1, Config{Interval: 24 * time.Hour, CheckInterval: time.Hour})
	s.sub.set([]subArtist{{ID: "s1", Name: "Renamed", MBzID: "a1"}}, nil)
	require.NoError(t, s.db.MarkArtistsChecked(ctx, []string{"a1"}, s.now))

	require.NoError(t, s.tasks.syncLibrary(ctx))

	a, err := s.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.Equal(t, "Renamed", a.Name)
	require.Equal(t, s.now.Unix(), *a.CheckedAt)
}

func TestStartContextChecksDueOnCheckTick(t *testing.T) {
	s := newSchedEnv(t, 2, Config{Enabled: true, Interval: time.Hour, CheckInterval: 20 * time.Millisecond})
	s.sub.set([]subArtist{{ID: "s1", Name: "Artist 1", MBzID: "a1"}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.tasks.StartContext(ctx)

	require.Eventually(t, func() bool {
		return s.mb.artistRequests("a1") >= 1 && s.mb.artistRequests("a2") >= 1
	}, 5*time.Second, 10*time.Millisecond)
}

// simulate runs ticks checkDue calls, advancing the clock by the check interval.
func simulate(t *testing.T, s *schedEnv, cfg Config, ticks int) {
	for range ticks {
		require.NoError(t, s.tasks.checkDue(context.Background(), s.now))
		s.now = s.now.Add(cfg.checkInterval())
	}
}

func TestCheckDueDrainsBacklogWithinInterval(t *testing.T) {
	const n = 10
	cfg := Config{Interval: 24 * time.Hour, CheckInterval: 3 * time.Hour}
	ticks := int(cfg.Interval / cfg.CheckInterval)

	for name, gap := range map[string]time.Duration{
		"fresh deploy": 0,
		"long gap":     5 * 24 * time.Hour,
	} {
		t.Run(name, func(t *testing.T) {
			s := newSchedEnv(t, n, cfg)
			if gap > 0 {
				// everyone checked once, then downtime
				simulate(t, s, cfg, ticks)
				s.now = s.now.Add(gap)
			}
			base := s.mb.requestCount()

			simulate(t, s, cfg, ticks)
			require.Equal(t, base+n, s.mb.requestCount())

			// steady state: once per interval again
			simulate(t, s, cfg, ticks)
			require.Equal(t, base+2*n, s.mb.requestCount())
			simulate(t, s, cfg, ticks)
			require.Equal(t, base+3*n, s.mb.requestCount())
		})
	}
}

func TestCheckDueNoCheckIntervalChecksAllEveryInterval(t *testing.T) {
	ctx := context.Background()
	cfg := Config{Interval: time.Hour}
	s := newSchedEnv(t, 5, cfg)

	require.NoError(t, s.tasks.checkDue(ctx, s.now))
	require.Equal(t, 5, s.mb.requestCount())

	s.now = s.now.Add(time.Hour + time.Second)
	require.NoError(t, s.tasks.checkDue(ctx, s.now))
	require.Equal(t, 10, s.mb.requestCount())
}

func TestStartContextNoCheckIntervalStillChecks(t *testing.T) {
	s := newSchedEnv(t, 2, Config{Enabled: true, Interval: 30 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.tasks.StartContext(ctx)

	require.Eventually(t, func() bool {
		return s.mb.artistRequests("a1") >= 1 && s.mb.artistRequests("a2") >= 1
	}, 5*time.Second, 10*time.Millisecond)
}

func TestCheckDueWithoutSubsonicStillChecksReleases(t *testing.T) {
	ctx := context.Background()
	s := newSchedEnv(t, 2, Config{Interval: time.Hour, CheckInterval: time.Hour})
	// Navidrome is down: the in-library step cannot resolve its clients
	s.navi.Close()

	require.NoError(t, s.tasks.checkDue(ctx, s.now))

	require.Equal(t, 1, s.mb.artistRequests("a1"))
	require.Equal(t, 1, s.mb.artistRequests("a2"))
	require.NotNil(t, s.checkedAt(t, "a1"))
	require.NotNil(t, s.checkedAt(t, "a2"))
}
