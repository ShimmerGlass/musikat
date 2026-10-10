package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/shimmerglass/musikat/server/auth"
	"github.com/stretchr/testify/require"
)

func TestSetupPage(t *testing.T) {
	e := newEnv(t)
	rec := e.do(e.s.setupPage, http.MethodGet, "/setup", nil, nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	e.addUser("alice", "Alice", "pw")
	rec = e.do(e.s.setupPage, http.MethodGet, "/setup", nil, nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/", rec.Header().Get("Location"))
}

func TestSetupCreatesFirstUser(t *testing.T) {
	e := newEnv(t)
	rec := e.do(e.s.setupHandler, http.MethodPost, "/setup", url.Values{
		"login": {"admin"}, "name": {"Admin"}, "password": {"pw"}, "confirm_password": {"pw"},
	}, nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/", rec.Header().Get("Location"))

	u, err := e.db.User(e.ctx, "admin")
	require.NoError(t, err)
	require.Equal(t, "Admin", u.Name)
	require.NotEqual(t, "pw", u.Password, "password must be hashed")
	ok, err := auth.ComparePasswordAndHash("pw", u.Password)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestSetupPasswordMismatch(t *testing.T) {
	e := newEnv(t)
	rec := e.do(e.s.setupHandler, http.MethodPost, "/setup", url.Values{
		"login": {"admin"}, "name": {"Admin"}, "password": {"pw"}, "confirm_password": {"other"},
	}, nil, nil)
	require.Empty(t, rec.Header().Get("Location"))
	require.Contains(t, rec.Body.String(), "passwords do not match")
	users, err := e.db.Users(e.ctx)
	require.NoError(t, err)
	require.Empty(t, users)
}

func TestSetupPostDisabledOnceUsersExist(t *testing.T) {
	e := newEnv(t)
	orig := e.addUser("alice", "Alice", "pw")
	e.do(e.s.setupHandler, http.MethodPost, "/setup", url.Values{
		"login": {"alice"}, "name": {"Evil"}, "password": {"x"}, "confirm_password": {"x"},
	}, nil, nil)
	u, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, orig, u)
}

func TestSetupEmptyLoginOrPassword(t *testing.T) {
	e := newEnv(t)
	e.do(e.s.setupHandler, http.MethodPost, "/setup", url.Values{}, nil, nil)
	users, err := e.db.Users(e.ctx)
	require.NoError(t, err)
	require.Empty(t, users)
}

func TestSetupCreatesAdmin(t *testing.T) {
	e := newEnv(t)
	rec := e.do(e.s.setupHandler, http.MethodPost, "/setup", url.Values{
		"login": {" root "}, "name": {"Root"}, "password": {"pw"}, "confirm_password": {"pw"},
	}, nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	u, err := e.db.User(e.ctx, "root")
	require.NoError(t, err)
	require.True(t, u.Admin)

	// A second setup attempt is refused.
	rec = e.do(e.s.setupHandler, http.MethodPost, "/setup", url.Values{
		"login": {"eve"}, "password": {"pw"}, "confirm_password": {"pw"},
	}, nil, nil)
	require.Equal(t, http.StatusForbidden, rec.Code)
}
