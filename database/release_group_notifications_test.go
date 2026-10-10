package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddReleaseGroupNotificationIdempotent(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))

		got, err := db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Equal(t, map[string]struct{}{"r1": {}}, got)
	})
}

func TestNotificationsPerUser(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u2"))

		got, err := db.NotifiedReleaseGroupIDs(ctx, "u2")
		require.NoError(t, err)
		require.Equal(t, map[string]struct{}{"r1": {}}, got)

		got, err = db.NotifiedReleaseGroupIDs(ctx, "nobody")
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})
}

func TestDeleteReleaseGroupNotificationScoped(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r2", "u1"))

		// deleting a missing notification is not an error
		require.NoError(t, db.DeleteReleaseGroupNotification(ctx, "nope", "u1"))
		require.NoError(t, db.DeleteReleaseGroupNotification(ctx, "r1", "other"))

		got, err := db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Len(t, got, 2)

		require.NoError(t, db.DeleteReleaseGroupNotification(ctx, "r1", "u1"))
		got, err = db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Equal(t, map[string]struct{}{"r2": {}}, got)

		// can be re-added after deletion
		require.NoError(t, db.AddReleaseGroupNotification(ctx, "r1", "u1"))
		got, err = db.NotifiedReleaseGroupIDs(ctx, "u1")
		require.NoError(t, err)
		require.Len(t, got, 2)
	})
}
