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
	e.addAdmin("alice", "Alice", "pw")
	e.addUser("bob", "Bobby", "pw")
	cookie := e.login("alice", "pw")

	rec := e.adminAuthed(e.s.users, cookie, http.MethodGet, "/admin/users", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Bobby")

	rec = e.adminAuthed(e.s.userCreatePage, cookie, http.MethodGet, "/admin/users/create", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestUserCreate(t *testing.T) {
	e := newEnv(t)
	e.addAdmin("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	rec := e.adminAuthed(e.s.userCreateHandler, cookie, http.MethodPost, "/admin/users/create",
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
	e.addAdmin("alice", "Alice", "pw")
	e.addUser("bob", "Bob", "pw")
	cookie := e.login("alice", "pw")

	rec := e.adminAuthed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"bob"}}, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/admin/users", rec.Header().Get("Location"))

	users, err := e.db.Users(e.ctx)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, "alice", users[0].ID)
}

func TestUserDeleteSelfRefused(t *testing.T) {
	e := newEnv(t)
	e.addAdmin("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	rec := e.adminAuthed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"alice"}}, nil)
	require.Empty(t, rec.Header().Get("Location"))
	require.Contains(t, rec.Body.String(), "cannot delete self")
	_, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
}

func TestUserManagementRequiresLogin(t *testing.T) {
	e := newEnv(t)
	e.addAdmin("alice", "Alice", "pw")
	e.addUser("bob", "Bob", "pw")

	rec := e.adminAuthed(e.s.userDelete, nil, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"bob"}}, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "/login")
	_, err := e.db.User(e.ctx, "bob")
	require.NoError(t, err)

	rec = e.adminAuthed(e.s.userCreateHandler, nil, http.MethodPost, "/admin/users/create",
		url.Values{"login": {"eve"}, "password": {"x"}}, nil)
	require.Contains(t, rec.Header().Get("Location"), "/login")
	_, err = e.db.User(e.ctx, "eve")
	require.ErrorIs(t, err, database.ErrUserNotFound)
}

func TestUserManagementIsAdminOnly(t *testing.T) {
	e := newEnv(t)
	e.addAdmin("alice", "Alice", "pw")
	e.addUser("bob", "Bob", "pw")
	cookie := e.login("bob", "pw") // not an admin

	rec := e.adminAuthed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"alice"}}, nil)
	require.Equal(t, http.StatusForbidden, rec.Code)
	_, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)

	rec = e.adminAuthed(e.s.userCreateHandler, cookie, http.MethodPost, "/admin/users/create",
		url.Values{"login": {"eve"}, "password": {"x"}, "admin": {"on"}}, nil)
	require.Equal(t, http.StatusForbidden, rec.Code)
	_, err = e.db.User(e.ctx, "eve")
	require.ErrorIs(t, err, database.ErrUserNotFound)

	rec = e.adminAuthed(e.s.users, cookie, http.MethodGet, "/admin/users", nil, nil)
	require.Equal(t, http.StatusForbidden, rec.Code)
	rec = e.adminAuthed(e.s.userCreatePage, cookie, http.MethodGet, "/admin/users/create", nil, nil)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestUserCreateValidation(t *testing.T) {
	e := newEnv(t)
	e.addAdmin("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	for name, form := range map[string]url.Values{
		"empty login":    {"login": {"  "}, "password": {"x"}},
		"empty password": {"login": {"eve"}, "password": {""}},
		"existing user":  {"login": {"alice"}, "password": {"x"}},
	} {
		t.Run(name, func(t *testing.T) {
			rec := e.adminAuthed(e.s.userCreateHandler, cookie, http.MethodPost, "/admin/users/create", form, nil)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Empty(t, rec.Header().Get("Location"))
		})
	}

	u, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, "Alice", u.Name)
	users, err := e.db.Users(e.ctx)
	require.NoError(t, err)
	require.Len(t, users, 1)
}

func TestUserCreateAdminFlag(t *testing.T) {
	e := newEnv(t)
	e.addAdmin("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	e.adminAuthed(e.s.userCreateHandler, cookie, http.MethodPost, "/admin/users/create",
		url.Values{"login": {"bob"}, "password": {"x"}, "admin": {"on"}}, nil)
	e.adminAuthed(e.s.userCreateHandler, cookie, http.MethodPost, "/admin/users/create",
		url.Values{"login": {"carl"}, "password": {"x"}}, nil)

	bob, err := e.db.User(e.ctx, "bob")
	require.NoError(t, err)
	require.True(t, bob.Admin)
	carl, err := e.db.User(e.ctx, "carl")
	require.NoError(t, err)
	require.False(t, carl.Admin)
}

func TestUserDeleteAdminAndMissing(t *testing.T) {
	e := newEnv(t)
	e.addAdmin("alice", "Alice", "pw")
	e.addAdmin("bob", "Bob", "pw")
	e.addUser("carl", "Carl", "pw")
	cookie := e.login("alice", "pw")

	// Two admins: deleting one is fine.
	rec := e.adminAuthed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"bob"}}, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)

	// Unknown user.
	rec = e.adminAuthed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"ghost"}}, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// A non-admin can be deleted; admins are never left at zero.
	rec = e.adminAuthed(e.s.userDelete, cookie, http.MethodPost, "/admin/users/delete", url.Values{"user_id": {"carl"}}, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
}
