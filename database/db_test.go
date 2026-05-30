package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

	do(context.Background(), db)
}
