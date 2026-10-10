package task

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
)

type plainTask struct {
	mu    sync.Mutex
	calls *[]string
	name  string
	err   error
	ran   chan struct{}
}

func (p *plainTask) Run(context.Context) error {
	p.mu.Lock()
	*p.calls = append(*p.calls, p.name+".Run")
	p.mu.Unlock()
	if p.ran != nil {
		p.ran <- struct{}{}
	}
	return p.err
}

type perArtistTask struct {
	plainTask
}

func (p *perArtistTask) RunArtist(_ context.Context, a database.Artist) error {
	p.mu.Lock()
	*p.calls = append(*p.calls, p.name+".RunArtist:"+a.Name)
	p.mu.Unlock()
	return p.err
}

func TestToSnakeCase(t *testing.T) {
	for in, want := range map[string]string{
		"RefreshArtists":        "refresh_artists",
		"RefreshInLibrary":      "refresh_in_library",
		"SendNotifications":     "send_notifications",
		"lower":                 "lower",
		"":                      "",
		"RefreshArtistReleases": "refresh_artist_releases",
	} {
		require.Equal(t, want, toSnakeCase(in), in)
	}
}

func TestTaskName(t *testing.T) {
	tasks := &Tasks{}
	require.Equal(t, "refresh_artists", tasks.taskName(&RefreshArtists{}))
	require.Equal(t, "refresh_in_library", tasks.taskName(&RefreshInLibrary{}))
	require.Equal(t, "send_notifications", tasks.taskName(&SendNotifications{}))
}

func TestNewTaskOrder(t *testing.T) {
	e := newEnv(t)
	tasks := New(Config{}, e.db, e.mbClient, e.subClient, e.naviClient, &fakeNotifier{})

	names := []string{}
	for _, tk := range tasks.tasks {
		names = append(names, tasks.taskName(tk))
	}
	require.Equal(t, []string{
		"refresh_artists", "refresh_starred", "refresh_artist_releases", "refresh_in_library", "send_notifications",
	}, names)
}

func TestTasksRunInOrderAndStopsOnError(t *testing.T) {
	calls := []string{}
	boom := errors.New("boom")
	tasks := &Tasks{tasks: []task{
		&plainTask{calls: &calls, name: "a"},
		&plainTask{calls: &calls, name: "b", err: boom},
		&plainTask{calls: &calls, name: "c"},
	}}

	err := tasks.Run(context.Background())

	require.ErrorIs(t, err, boom)
	require.ErrorContains(t, err, "plain_task: boom")
	require.Equal(t, []string{"a.Run", "b.Run"}, calls)
	require.False(t, tasks.Running())
}

func TestTasksRunArtistFansOut(t *testing.T) {
	calls := []string{}
	tasks := &Tasks{tasks: []task{
		&plainTask{calls: &calls, name: "plain"},
		&perArtistTask{plainTask{calls: &calls, name: "per"}},
	}}

	require.NoError(t, tasks.RunArtist(context.Background(), database.Artist{Name: "X"}))

	// tasks without RunArtist run in full, the others only for the artist
	require.Equal(t, []string{"plain.Run", "per.RunArtist:X"}, calls)
	require.False(t, tasks.Running())
}

func TestTasksRunArtistErrors(t *testing.T) {
	boom := errors.New("boom")
	for name, mk := range map[string]func(calls *[]string) []task{
		"plain": func(c *[]string) []task {
			return []task{&plainTask{calls: c, name: "p", err: boom}, &plainTask{calls: c, name: "after"}}
		},
		"per artist": func(c *[]string) []task {
			return []task{&perArtistTask{plainTask{calls: c, name: "p", err: boom}}, &plainTask{calls: c, name: "after"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			calls := []string{}
			tasks := &Tasks{tasks: mk(&calls)}

			err := tasks.RunArtist(context.Background(), database.Artist{Name: "X"})

			require.ErrorIs(t, err, boom)
			require.Len(t, calls, 1)
			require.False(t, tasks.Running())
		})
	}
}

func TestTasksRunning(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	tasks := &Tasks{tasks: []task{blockingTask{started: started, release: release}}}

	done := make(chan error)
	go func() { done <- tasks.Run(context.Background()) }()

	<-started
	require.True(t, tasks.Running())
	close(release)
	require.NoError(t, <-done)
	require.False(t, tasks.Running())
}

type blockingTask struct {
	started chan struct{}
	release chan struct{}
}

func (b blockingTask) Run(context.Context) error {
	close(b.started)
	<-b.release
	return nil
}

func TestTasksRefreshDoesNotBlockWithoutLoop(t *testing.T) {
	tasks := New(Config{}, nil, nil, nil, nil, nil)

	done := make(chan struct{})
	go func() {
		tasks.Refresh()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Refresh blocked")
	}
}

func TestTasksStartRefreshAndRefreshArtist(t *testing.T) {
	calls := []string{}
	ran := make(chan struct{}, 10)
	pt := &perArtistTask{plainTask{calls: &calls, name: "t", ran: ran}}
	tasks := &Tasks{
		refresh:       make(chan struct{}),
		refreshArtist: make(chan database.Artist, 32),
		tasks:         []task{pt},
	}
	tasks.Start() // Config.Enabled is false: nothing runs on its own

	select {
	case <-ran:
		t.Fatal("task ran although scheduling is disabled")
	case <-time.After(100 * time.Millisecond):
	}

	// RefreshArtist only runs the per-artist path
	tasks.RefreshArtist(database.Artist{Name: "X"})
	require.Eventually(t, func() bool {
		pt.mu.Lock()
		defer pt.mu.Unlock()
		return len(calls) == 1
	}, 2*time.Second, 5*time.Millisecond)
	pt.mu.Lock()
	require.Equal(t, "t.RunArtist:X", calls[0])
	pt.mu.Unlock()

	// Refresh drops the signal if the loop is busy, so retry until it lands
	require.Eventually(t, func() bool {
		tasks.Refresh()
		select {
		case <-ran:
			return true
		case <-time.After(20 * time.Millisecond):
			return false
		}
	}, 2*time.Second, 5*time.Millisecond)
}

func TestTasksStartSyncsLibraryImmediatelyAndOnInterval(t *testing.T) {
	calls := []string{}
	ran := make(chan struct{}, 100)
	tasks := &Tasks{
		cfg:           Config{Enabled: true, Interval: 50 * time.Millisecond},
		refresh:       make(chan struct{}),
		refreshArtist: make(chan database.Artist, 32),
		librarySync:   []task{&plainTask{calls: &calls, name: "t", ran: ran}},
	}
	tasks.Start()

	for i := 0; i < 3; i++ { // initial sync, then two ticks
		select {
		case <-ran:
		case <-time.After(2 * time.Second):
			t.Fatalf("run %d did not happen", i)
		}
	}
}

// TestTasksRunArtistEndToEnd runs the full pipeline for one artist against
// fake external services.
func TestTasksRunArtistEndToEnd(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.addUser(t, "u1")
	today := time.Now().UTC().Format("2006-01-02")
	e.sub.set([]subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}}, []subArtist{{ID: "s1", Name: "Artist", MBzID: "a1"}})
	e.mb.rgs["a1"] = []mbRG{
		{ID: "rg1", Title: "Fresh", Primary: "Album", Date: today, Artists: []database.Artist{artistA}},
		{ID: "rg2", Title: "Owned", Primary: "Album", Date: "2010-01-01", Artists: []database.Artist{artistA}},
	}
	e.navi.setSongs("s1", song("relOwned", "rg2", "r1"))
	n := &fakeNotifier{}

	tasks := New(Config{}, e.db, e.mbClient, e.subClient, e.naviClient, n)
	require.NoError(t, tasks.RunArtist(ctx, artistA))

	// starred artist is watched
	w, ok, err := e.db.ArtistWatch(ctx, "u1", "a1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "subsonic", w.Source)

	// release groups were fetched and their library state resolved
	rg1, err := e.db.ReleaseGroup(ctx, "rg1")
	require.NoError(t, err)
	require.Equal(t, database.LibraryStatusMissing, rg1.LibraryStatus)
	rg2, err := e.db.ReleaseGroup(ctx, "rg2")
	require.NoError(t, err)
	require.Equal(t, database.LibraryStatusPresent, rg2.LibraryStatus)
	require.Equal(t, "relOwned", rg2.InLibraryReleaseMBzID)

	a, err := e.db.Artist(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, a.RefreshedAt)

	// only the missing release is notified, once
	require.Len(t, n.msgs, 1)
	require.Contains(t, n.msgs[0], "Fresh")
	require.NoError(t, tasks.RunArtist(ctx, artistA))
	require.Len(t, n.msgs, 1)
}

func TestTasksRefreshArtistDoesNotBlockWithoutStart(t *testing.T) {
	tasks := New(Config{}, nil, nil, nil, nil, nil)

	done := make(chan struct{})
	go func() {
		for range 3 * refreshArtistBuffer { // more than the queue holds: extras are dropped
			tasks.RefreshArtist(database.Artist{Name: "X"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RefreshArtist blocked")
	}
	require.Len(t, tasks.refreshArtist, refreshArtistBuffer)
}

// gateTask signals on started and then waits for a value on release.
type gateTask struct {
	started chan struct{}
	release chan struct{}
}

func (g gateTask) Run(context.Context) error {
	g.started <- struct{}{}
	<-g.release
	return nil
}

func TestTasksRunningCounterOverlappingRuns(t *testing.T) {
	g := gateTask{started: make(chan struct{}), release: make(chan struct{})}
	tasks := &Tasks{tasks: []task{g}}

	done := make(chan error, 2)
	go func() { done <- tasks.Run(context.Background()) }()
	<-g.started
	go func() { done <- tasks.RunArtist(context.Background(), database.Artist{Name: "X"}) }()
	<-g.started
	require.True(t, tasks.Running())

	g.release <- struct{}{} // one of them finishes, the other still runs
	require.NoError(t, <-done)
	require.True(t, tasks.Running())

	g.release <- struct{}{}
	require.NoError(t, <-done)
	require.False(t, tasks.Running())
}

func TestTasksStartContextStops(t *testing.T) {
	ran := make(chan struct{}, 100)
	calls := []string{}
	tasks := &Tasks{
		cfg:           Config{Enabled: true, Interval: 10 * time.Millisecond},
		refresh:       make(chan struct{}),
		refreshArtist: make(chan database.Artist, 32),
		librarySync:   []task{&plainTask{calls: &calls, name: "t", ran: ran}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	tasks.StartContext(ctx)
	<-ran
	cancel()
	time.Sleep(50 * time.Millisecond)
	for len(ran) > 0 {
		<-ran
	}
	time.Sleep(50 * time.Millisecond)
	require.Empty(t, ran)
}
