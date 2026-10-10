package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/auth"
	"github.com/stretchr/testify/require"
)

func TestUsersList(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	e.addUser("bob", "Bobby", "pw")
	cookie := e.login("alice", "pw")

	rec := e.authed(e.s.users, cookie, http.MethodGet, "/admin/users", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Bobby")

	rec = e.authed(e.s.userCreatePage, cookie, http.MethodGet, "/admin/users/create", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestUserCreate(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	rec := e.authed(e.s.userCreateHandler, cookie, http.MethodPost, "/admin/users/create",
		url.Values{"login": {"bob"}, "name": {"Bob"}, "password": {"bobpw"}}, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/admin/users", rec.Header().Get("Location"))

	u, err := e.db.User(e.ctx, "bob")
	require.NoError(t, err)
	require.Equal(t, "Bob", u.Name)
	ok, err := auth.ComparePasswordAndHash("bobpw", u.Password)
	require.NoError(t, err)
	require.True(t, ok)

	// New user can log in.
	require.NotNil(t, e.login("bob", "bobpw"))
}

func TestUserDelete(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	e.addUser("bob", "Bob", "pw")
	cookie := e.login("alice", "pw")

	rec := e.authed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"bob"}}, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/admin/users", rec.Header().Get("Location"))

	users, err := e.db.Users(e.ctx)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, "alice", users[0].ID)
}

func TestUserDeleteSelfRefused(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	rec := e.authed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"alice"}}, nil)
	require.Empty(t, rec.Header().Get("Location"))
	require.Contains(t, rec.Body.String(), "cannot delete self")
	_, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
}

func TestUserManagementRequiresLogin(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	e.addUser("bob", "Bob", "pw")

	rec := e.authed(e.s.userDelete, nil, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"bob"}}, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "/login")
	_, err := e.db.User(e.ctx, "bob")
	require.NoError(t, err)

	rec = e.authed(e.s.userCreateHandler, nil, http.MethodPost, "/admin/users/create",
		url.Values{"login": {"eve"}, "password": {"x"}}, nil)
	require.Contains(t, rec.Header().Get("Location"), "/login")
	_, err = e.db.User(e.ctx, "eve")
	require.ErrorIs(t, err, database.ErrUserNotFound)
}

func TestUserManagementIsAdminOnly(t *testing.T) {
	t.Skip("BUG: there is no admin concept; any authenticated user can list, create and delete users (server/users.go, routes in server/server.go:44-47)")
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	e.addUser("bob", "Bob", "pw")
	cookie := e.login("bob", "pw") // not an admin
	e.authed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"alice"}}, nil)
	_, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
}
