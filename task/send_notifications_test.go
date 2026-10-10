package task

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	opt "github.com/shimmerglass/go-optional"
	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
)

func TestShouldNotify(t *testing.T) {
	now := time.Date(2026, time.October, 10, 15, 30, 0, 0, time.UTC)
	added := time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC).Unix()

	watch := func(mod func(*database.ArtistWatch)) database.ArtistWatch {
		w := database.ArtistWatch{
			Status:           true,
			AddedAt:          added,
			XXPrimaryTypes:   "Album,EP",
			XXSecondaryTypes: "",
		}
		if mod != nil {
			mod(&w)
		}
		return w
	}
	rg := func(date string, mod func(*database.ReleaseGroup)) database.ReleaseGroup {
		r := database.ReleaseGroup{
			ReleaseDate:   date,
			PrimaryType:   "Album",
			LibraryStatus: database.LibraryStatusMissing,
		}
		if mod != nil {
			mod(&r)
		}
		return r
	}

	for _, tc := range []struct {
		name  string
		rg    database.ReleaseGroup
		watch database.ArtistWatch
		want  bool
	}{
		{"full date past", rg("2026-09-01", nil), watch(nil), true},
		{"full date today", rg("2026-10-10", nil), watch(nil), true},
		{"full date future", rg("2026-10-11", nil), watch(nil), false},
		{"year only current year", rg("2026", nil), watch(nil), false},
		{"year only last year, watch older than release end", rg("2025", nil), watch(func(w *database.ArtistWatch) { w.AddedAt = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC).Unix() }), true},
		{"year only last year, before watch", rg("2025", nil), watch(nil), false},
		{"year-month current month", rg("2026-10", nil), watch(nil), false},
		{"year-month past, after watch", rg("2026-08", nil), watch(nil), true},
		{"year-month containing watch date", rg("2026-06", nil), watch(nil), true},
		{"year-month before watch", rg("2026-05", nil), watch(nil), false},
		{"same day as watch added", rg("2026-06-15", nil), watch(nil), true},
		{"day before watch added", rg("2026-06-14", nil), watch(nil), false},
		{"long before watch added", rg("2010-03-04", nil), watch(nil), false},
		{"watch disabled", rg("2026-09-01", nil), watch(func(w *database.ArtistWatch) { w.Status = false }), false},
		{"present in library", rg("2026-09-01", func(r *database.ReleaseGroup) { r.LibraryStatus = database.LibraryStatusPresent }), watch(nil), false},
		{"unknown status", rg("2026-09-01", func(r *database.ReleaseGroup) { r.LibraryStatus = database.LibraryStatusUnknown }), watch(nil), false},
		{"primary type not watched", rg("2026-09-01", func(r *database.ReleaseGroup) { r.PrimaryType = "Single" }), watch(nil), false},
		{"secondary type not watched", rg("2026-09-01", func(r *database.ReleaseGroup) { r.XXSecondaryTypes = "Live" }), watch(func(w *database.ArtistWatch) { w.XXSecondaryTypes = "Compilation" }), false},
		{"secondary type watched", rg("2026-09-01", func(r *database.ReleaseGroup) { r.XXSecondaryTypes = "Live" }), watch(func(w *database.ArtistWatch) { w.XXSecondaryTypes = "Live" }), true},
		{"empty date", rg("", nil), watch(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldNotify(tc.rg, tc.watch, now))
		})
	}
}

type fakeNotifier struct {
	mu     sync.Mutex
	fail   map[string]bool // by user ID
	sent   map[string]int  // by user ID
	failed int
	msgs   []string
}

type fakeCovers struct {
	url string
	err error
}

func (f fakeCovers) ReleaseGroupCoverURL(context.Context, string) (string, error) {
	return f.url, f.err
}

func (f *fakeNotifier) Send(_ context.Context, to database.User, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail[to.ID] {
		f.failed++
		return errors.New("boom")
	}
	if f.sent == nil {
		f.sent = map[string]int{}
	}
	f.sent[to.ID]++
	f.msgs = append(f.msgs, msg)
	return nil
}

func newTestDB(t *testing.T) *database.DB {
	db, err := database.New(database.Config{Path: filepath.Join(t.TempDir(), "rw.db")})
	require.NoError(t, err)
	return db
}

func seedWatch(t *testing.T, ctx context.Context, db *database.DB, userID string) {
	require.NoError(t, db.AddUser(ctx, database.User{ID: userID, Name: userID}))
	require.NoError(t, db.AddArtistWatch(ctx, database.ArtistWatch{
		UserID:         userID,
		ArtistMBzID:    "a1",
		Status:         true,
		AddedAt:        time.Now().Add(-24 * time.Hour).Unix(),
		XXPrimaryTypes: "Album",
	}))
}

func seedMissingRG(t *testing.T, ctx context.Context, db *database.DB) {
	artist := database.Artist{MBzID: "a1", Name: "Artist"}
	require.NoError(t, db.PutArtist(ctx, "a1", func(opt.Option[database.Artist]) database.Artist { return artist }))
	rg := database.ReleaseGroup{
		MBzID:         "rg1",
		Name:          "RG",
		PrimaryType:   "Album",
		ReleaseDate:   time.Now().UTC().Format("2006-01-02"),
		LibraryStatus: database.LibraryStatusMissing,
		Artists:       []database.Artist{artist},
	}
	require.NoError(t, db.PutReleaseGroup(ctx, "rg1", func(opt.Option[database.ReleaseGroup]) database.ReleaseGroup { return rg }))
	require.NoError(t, db.ReplaceReleaseGroupArtists(ctx, rg))
}

func TestSendNotificationsRetriesAfterFailure(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedWatch(t, ctx, db, "u1")
	seedMissingRG(t, ctx, db)

	n := &fakeNotifier{fail: map[string]bool{"u1": true}}
	task := NewSendNotifications(db, n, fakeCovers{})

	require.NoError(t, task.Run(ctx))
	require.Equal(t, 1, n.failed)
	got, err := db.NotifiedReleaseGroupIDs(ctx, "u1")
	require.NoError(t, err)
	require.Empty(t, got)

	// next run retries and succeeds
	n.fail = nil
	require.NoError(t, task.Run(ctx))
	require.Equal(t, 1, n.sent["u1"])
	got, err = db.NotifiedReleaseGroupIDs(ctx, "u1")
	require.NoError(t, err)
	require.Contains(t, got, "rg1")

	// and is not re-sent afterwards
	require.NoError(t, task.Run(ctx))
	require.Equal(t, 1, n.sent["u1"])
}

func TestSendNotificationsSkipsUnwatchedArtist(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedWatch(t, ctx, db, "u1")
	seedMissingRG(t, ctx, db)
	require.NoError(t, db.AddArtistWatch(ctx, database.ArtistWatch{
		UserID: "u1", ArtistMBzID: "a1", Status: false, XXPrimaryTypes: "Album",
	}))

	n := &fakeNotifier{}
	require.NoError(t, NewSendNotifications(db, n, fakeCovers{}).Run(ctx))
	require.Empty(t, n.sent)
}

func TestSendNotificationsUserFailureDoesNotStopOthers(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedWatch(t, ctx, db, "u1")
	seedWatch(t, ctx, db, "u2")
	seedMissingRG(t, ctx, db)

	n := &fakeNotifier{fail: map[string]bool{"u1": true}}
	require.NoError(t, NewSendNotifications(db, n, fakeCovers{}).Run(ctx))
	require.Equal(t, 1, n.sent["u2"])
	require.Zero(t, n.sent["u1"])
}

func TestSendNotificationsRunReturnsUserErrors(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedWatch(t, ctx, db, "u1")
	seedWatch(t, ctx, db, "u2")
	seedMissingRG(t, ctx, db)

	// u1's watch points at an artist that doesn't exist: runUser errors
	require.NoError(t, db.AddArtistWatch(ctx, database.ArtistWatch{
		UserID: "u1", ArtistMBzID: "ghost", Status: true, XXPrimaryTypes: "Album",
	}))

	n := &fakeNotifier{}
	err := NewSendNotifications(db, n, fakeCovers{}).Run(ctx)
	require.Error(t, err)
	require.Equal(t, 1, n.sent["u2"])
}

func TestSendNotificationsMessage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		covers fakeCovers
		want   string
	}{
		{"cover found", fakeCovers{url: "https://img/c.jpg"}, "New release from Artist!\nRG (Album)\nhttps://musicbrainz.org/release-group/rg1\nhttps://img/c.jpg"},
		{"no cover", fakeCovers{}, "New release from Artist!\nRG (Album)\nhttps://musicbrainz.org/release-group/rg1"},
		{"lookup error", fakeCovers{err: errors.New("down")}, "New release from Artist!\nRG (Album)\nhttps://musicbrainz.org/release-group/rg1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB(t)
			seedWatch(t, ctx, db, "u1")
			seedMissingRG(t, ctx, db)

			n := &fakeNotifier{}
			require.NoError(t, NewSendNotifications(db, n, tc.covers).Run(ctx))
			require.Equal(t, []string{tc.want}, n.msgs)

			got, err := db.NotifiedReleaseGroupIDs(ctx, "u1")
			require.NoError(t, err)
			require.Contains(t, got, "rg1")
		})
	}
}

func TestMarkAllMissing(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedMissingRG(t, ctx, db)
	require.NoError(t, db.PutReleaseGroup(ctx, "rg1", func(o opt.Option[database.ReleaseGroup]) database.ReleaseGroup {
		rg := o.TakeOr(database.ReleaseGroup{})
		rg.LibraryStatus = database.LibraryStatusUnknown
		rg.InLibraryReleaseMBzID = "stale"
		return rg
	}))
	rgs, err := db.ArtistReleaseGroups(ctx, database.Artist{MBzID: "a1"})
	require.NoError(t, err)

	task := &RefreshInLibrary{db: db}
	require.NoError(t, task.markAllMissing(ctx, rgs))

	rg, err := db.ReleaseGroup(ctx, "rg1")
	require.NoError(t, err)
	require.Equal(t, database.LibraryStatusMissing, rg.LibraryStatus)
	require.Empty(t, rg.InLibraryReleaseMBzID)
}
