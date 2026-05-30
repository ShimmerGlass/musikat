package database

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUserCreate(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		u := User{ID: uuid.New().String(), Name: "Alice"}
		err := db.AddUser(ctx, u)
		require.NoError(t, err)

		users, err := db.ListUsers(ctx)
		require.NoError(t, err)

		require.Len(t, users, 1)
		require.Equal(t, users[0], u)
	})
}

func TestUserUpdate(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		u := User{ID: uuid.New().String(), Name: "Alice"}
		err := db.AddUser(ctx, u)
		require.NoError(t, err)

		u.Name = "Bob"
		err = db.AddUser(ctx, u)
		require.NoError(t, err)

		users, err := db.ListUsers(ctx)
		require.NoError(t, err)

		require.Len(t, users, 1)
		require.Equal(t, users[0], u)
	})
}
