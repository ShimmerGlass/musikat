package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshDefaultRedirect(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	rec := e.authed(e.s.refresh, cookie, http.MethodGet, "/refresh", nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/", rec.Header().Get("Location"))
}

func TestRefreshNextRedirect(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	rec := e.authed(e.s.refresh, cookie, http.MethodGet, "/refresh?next=%2Faccount", nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/account", rec.Header().Get("Location"))
}

func TestRefreshUnknownArtist(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	rec := e.authed(e.s.refresh, cookie, http.MethodGet, "/refresh?artist=ghost", nil, nil)
	require.Empty(t, rec.Header().Get("Location"))
	require.Contains(t, rec.Body.String(), "artist not found")
}

func TestRefreshRequiresLogin(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	rec := e.authed(e.s.refresh, nil, http.MethodGet, "/refresh", nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "/login")
}

// Refreshing a single known artist is not tested: Tasks.RefreshArtist blocks on
// an unbuffered channel that is only drained by Tasks.Start, which would run the
// real (network-bound) tasks.
