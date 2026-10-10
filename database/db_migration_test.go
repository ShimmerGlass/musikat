package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFreshDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	db, err := New(Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.sql.Close() })

	ctx := context.Background()
	for _, table := range []string{
		tableUsers, tableArtists, tableArtistWatches, tableReleaseGroups,
		tableReleaseGroupArtists, tableReleaseGroupNotifications, tableReleaseGroupReleases,
	} {
		var n int
		require.NoError(t, db.sql.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n), table)
		require.Equal(t, 0, n, table)
	}

	var dirty bool
	var version int64
	require.NoError(t, db.sql.QueryRowContext(ctx, "SELECT version, dirty FROM migrations").Scan(&version, &dirty))
	require.False(t, dirty)
	require.Positive(t, version)
}

func TestNewReopenIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	ctx := context.Background()

	db, err := New(Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, db.CreateUser(ctx, User{ID: "u1", Name: "Alice"}))
	require.NoError(t, db.addArtist(ctx, Artist{MBzID: "a1", Name: "A"}))
	var v1 int64
	require.NoError(t, db.sql.QueryRowContext(ctx, "SELECT version FROM migrations").Scan(&v1))
	require.NoError(t, db.sql.Close())

	db2, err := New(Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db2.sql.Close() })

	// data survives and the schema version is unchanged
	u, err := db2.User(ctx, "u1")
	require.NoError(t, err)
	require.Equal(t, "Alice", u.Name)
	a, err := db2.Artist(ctx, "a1")
	require.NoError(t, err)
	require.Equal(t, "A", a.Name)

	var v2 int64
	var rows int
	require.NoError(t, db2.sql.QueryRowContext(ctx, "SELECT version FROM migrations").Scan(&v2))
	require.NoError(t, db2.sql.QueryRowContext(ctx, "SELECT count(*) FROM migrations").Scan(&rows))
	require.Equal(t, v1, v2)
	require.Equal(t, 1, rows)
}

func TestNewInvalidPath(t *testing.T) {
	// parent directory does not exist
	_, err := New(Config{Path: filepath.Join(t.TempDir(), "missing", "x.db")})
	require.Error(t, err)
}
