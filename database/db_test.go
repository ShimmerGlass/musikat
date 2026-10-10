package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/samber/lo"
	opt "github.com/shimmerglass/go-optional"
	"github.com/stretchr/testify/require"
)

func WithDB(t *testing.T, do func(context.Context, *DB)) {
	tmpdir, err := os.MkdirTemp(os.TempDir(), "db")
	require.NoError(t, err)

	defer func() { _ = os.RemoveAll(tmpdir) }()

	db, err := New(Config{
		Path: filepath.Join(tmpdir, "rw.db"),
	})
	require.NoError(t, err)

	defer func() { require.NoError(t, db.Close()) }()

	do(context.Background(), db)
}

func TestPutConcurrentNoLostUpdates(t *testing.T) {
	const n = 20

	WithDB(t, func(ctx context.Context, db *DB) {
		var wg sync.WaitGroup
		errs := make(chan error, 2*n)
		for range n {
			wg.Go(func() {
				errs <- db.PutArtist(ctx, "a1", func(o opt.Option[Artist]) Artist {
					a := o.TakeOr(Artist{MBzID: "a1", Name: "x"})
					a.RefreshedAt = new(lo.FromPtr(a.RefreshedAt) + 1)
					return a
				})
			})
			wg.Go(func() {
				errs <- db.PutReleaseGroup(ctx, "r1", func(o opt.Option[ReleaseGroup]) ReleaseGroup {
					rg := o.TakeOr(ReleaseGroup{MBzID: "r1", Name: "x"})
					rg.InLibraryReleaseMBzID = strings.Repeat("x", len(rg.InLibraryReleaseMBzID)+1)
					return rg
				})
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}

		a, err := db.Artist(ctx, "a1")
		require.NoError(t, err)
		require.EqualValues(t, n, lo.FromPtr(a.RefreshedAt))

		rg, err := db.ReleaseGroup(ctx, "r1")
		require.NoError(t, err)
		require.Len(t, rg.InLibraryReleaseMBzID, n)
	})
}
