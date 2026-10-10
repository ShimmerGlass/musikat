package database

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUserCreate(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		u := User{ID: uuid.New().String(), Name: "Alice"}
		err := db.CreateUser(ctx, u)
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
		err := db.CreateUser(ctx, u)
		require.NoError(t, err)

		u.Name = "Bob"
		err = db.UpdateUser(ctx, u)
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
		require.NoError(t, db.CreateUser(ctx, u1))
		require.NoError(t, db.CreateUser(ctx, u2))

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
		require.NoError(t, db.CreateUser(ctx, u))

		u = User{
			ID: "u1", Name: "Alice2", Password: "pw2",
			SubsonicUser: "su2", SubsonicPass: "sp2", XMPPJID: "b@x", Admin: true,
		}
		require.NoError(t, db.UpdateUser(ctx, u))

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
		require.NoError(t, db.CreateUser(ctx, User{ID: "u1", Name: "Alice"}))
		require.NoError(t, db.CreateUser(ctx, User{ID: "u2", Name: "Bob"}))

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

func TestUserCreateDuplicate(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.CreateUser(ctx, User{ID: "u1", Name: "Alice", Password: "pw"}))
		err := db.CreateUser(ctx, User{ID: "u1", Name: "Evil", Password: "x"})
		require.ErrorIs(t, err, ErrUserExists)

		got, err := db.User(ctx, "u1")
		require.NoError(t, err)
		require.Equal(t, "Alice", got.Name)
	})
}

func TestCreateFirstUser(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.CreateFirstUser(ctx, User{ID: "admin", Name: "Admin", Admin: true}))

		err := db.CreateFirstUser(ctx, User{ID: "evil", Name: "Evil"})
		require.ErrorIs(t, err, ErrSetupDone)
		err = db.CreateFirstUser(ctx, User{ID: "admin", Name: "Evil"})
		require.ErrorIs(t, err, ErrSetupDone)

		got, err := db.User(ctx, "admin")
		require.NoError(t, err)
		require.Equal(t, "Admin", got.Name)
		require.True(t, got.Admin)
	})
}

func TestCreateFirstUserConcurrent(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = db.CreateFirstUser(ctx, User{ID: fmt.Sprintf("u%d", i)})
			}()
		}
		wg.Wait()

		users, err := db.Users(ctx)
		require.NoError(t, err)
		require.Len(t, users, 1)
	})
}

func TestUserUpdateMissing(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		err := db.UpdateUser(ctx, User{ID: "nope", Name: "X"})
		require.ErrorIs(t, err, ErrUserNotFound)
	})
}

func TestUserDeleteRemovesWatchesAndNotifications(t *testing.T) {
	WithDB(t, func(ctx context.Context, db *DB) {
		require.NoError(t, db.CreateUser(ctx, User{ID: "u1", Name: "Alice"}))
		require.NoError(t, db.CreateUser(ctx, User{ID: "u2", Name: "Bob"}))
		for _, u := range []string{"u1", "u2"} {
			_, err := db.sql.ExecContext(ctx, "INSERT INTO release_group_notifications (user_id, release_group_mb_id) VALUES (?, 'rg')", u)
			require.NoError(t, err)
		}

		require.NoError(t, db.UserDelete(ctx, "u1"))

		var n int
		require.NoError(t, db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM release_group_notifications").Scan(&n))
		require.Equal(t, 1, n)
		require.NoError(t, db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM release_group_notifications WHERE user_id = 'u1'").Scan(&n))
		require.Zero(t, n)
	})
}
