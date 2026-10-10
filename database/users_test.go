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

		users, err := db.Users(ctx)
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

		users, err := db.Users(ctx)
		require.NoError(t, err)

		require.Len(t, users, 1)
		require.Equal(t, users[0], u)
	})
}

func TestUserByID(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		u1 := User{
			ID: "u1", Name: "Alice", Password: "pw",
			SubsonicUser: "su", SubsonicPass: "sp", XMPPJID: "alice@example.org",
		}
		u2 := User{ID: "u2", Name: "Bob"}
		require.NoError(t, db.AddUser(ctx, u1))
		require.NoError(t, db.AddUser(ctx, u2))

		got, err := db.User(ctx, "u1")
		require.NoError(t, err)
		require.Equal(t, u1, got)

		got, err = db.User(ctx, "u2")
		require.NoError(t, err)
		require.Equal(t, u2, got)

		got, err = db.User(ctx, "missing")
		require.ErrorIs(t, err, ErrUserNotFound)
		require.Equal(t, User{}, got)
	})
}

func TestUsersEmpty(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		users, err := db.Users(ctx)
		require.NoError(t, err)
		require.NotNil(t, users)
		require.Empty(t, users)
	})
}

func TestUserUpdateAllFields(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		u := User{
			ID: "u1", Name: "Alice", Password: "pw",
			SubsonicUser: "su", SubsonicPass: "sp", XMPPJID: "a@x",
		}
		require.NoError(t, db.AddUser(ctx, u))

		u = User{
			ID: "u1", Name: "Alice2", Password: "pw2",
			SubsonicUser: "su2", SubsonicPass: "sp2", XMPPJID: "b@x",
		}
		require.NoError(t, db.AddUser(ctx, u))

		got, err := db.User(ctx, "u1")
		require.NoError(t, err)
		require.Equal(t, u, got)

		users, err := db.Users(ctx)
		require.NoError(t, err)
		require.Len(t, users, 1)
	})
}

func TestUserDelete(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.AddUser(ctx, User{ID: "u1", Name: "Alice"}))
		require.NoError(t, db.AddUser(ctx, User{ID: "u2", Name: "Bob"}))

		require.NoError(t, db.UserDelete(ctx, "u1"))

		_, err := db.User(ctx, "u1")
		require.ErrorIs(t, err, ErrUserNotFound)
		_, err = db.User(ctx, "u2")
		require.NoError(t, err)

		// deleting a missing user is not an error
		require.NoError(t, db.UserDelete(ctx, "u1"))
		require.NoError(t, db.UserDelete(ctx, "nope"))

		users, err := db.Users(ctx)
		require.NoError(t, err)
		require.Len(t, users, 1)
	})
}
