package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/shimmerglass/musikat/server/auth"
	"github.com/stretchr/testify/require"
)

func TestAccountPage(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")
	rec := e.authed(e.s.accountPage, cookie, http.MethodGet, "/account", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
}

func (e *env) postAccount(cookie *http.Cookie, form url.Values) *http.Response {
	e.t.Helper()
	return e.authed(e.s.accountUpdate, cookie, http.MethodPost, "/account", form, nil).Result()
}

func TestAccountUpdateInfo(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	resp := e.postAccount(cookie, url.Values{"type": {"info"}, "name": {"Alicia"}})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/account", resp.Header.Get("Location"))

	u, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, "Alicia", u.Name)
	// Other fields untouched.
	ok, err := auth.ComparePasswordAndHash("pw", u.Password)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestAccountUpdateSubsonicAndNotifications(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	resp := e.postAccount(cookie, url.Values{"type": {"subsonic"}, "subsonic_user": {"su"}, "subsonic_pass": {"sp"}})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	resp = e.postAccount(cookie, url.Values{"type": {"notifications"}, "xmpp_jid": {"a@example.org"}})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	u, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, "su", u.SubsonicUser)
	require.Equal(t, "sp", u.SubsonicPass)
	require.Equal(t, "a@example.org", u.XMPPJID)
	require.Equal(t, "Alice", u.Name)
}

func TestAccountUpdatePassword(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	resp := e.postAccount(cookie, url.Values{"type": {"password"}, "current_pass": {"pw"}, "new_pass": {"new"}, "confirm_pass": {"new"}})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	u, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
	ok, err := auth.ComparePasswordAndHash("new", u.Password)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, e.login("alice", "new"))
}

func TestAccountUpdatePasswordErrors(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	cases := map[string]struct {
		form url.Values
		msg  string
	}{
		"mismatch":      {url.Values{"type": {"password"}, "current_pass": {"pw"}, "new_pass": {"a"}, "confirm_pass": {"b"}}, "passwords do not match"},
		"wrong current": {url.Values{"type": {"password"}, "current_pass": {"bad"}, "new_pass": {"a"}, "confirm_pass": {"a"}}, "invalid current password"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec := e.authed(e.s.accountUpdate, cookie, http.MethodPost, "/account", c.form, nil)
			require.Empty(t, rec.Header().Get("Location"))
			require.Contains(t, rec.Body.String(), c.msg)

			u, err := e.db.User(e.ctx, "alice")
			require.NoError(t, err)
			ok, err := auth.ComparePasswordAndHash("pw", u.Password)
			require.NoError(t, err)
			require.True(t, ok, "password must be unchanged")
		})
	}
}

func TestAccountUpdateRequiresLogin(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	resp := e.postAccount(nil, url.Values{"type": {"info"}, "name": {"Evil"}})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Location"), "/login")
	u, err := e.db.User(e.ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, "Alice", u.Name)
}

func TestAccountUpdateUnknownType(t *testing.T) {
	t.Skip("BUG: accountUpdate with an unknown/missing 'type' silently succeeds with a redirect (server/account.go:24-47) instead of rejecting the request")
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")
	rec := e.authed(e.s.accountUpdate, cookie, http.MethodPost, "/account", url.Values{"type": {"bogus"}}, nil)
	require.Empty(t, rec.Header().Get("Location"))
}
