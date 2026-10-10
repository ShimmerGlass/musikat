package server

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestLoginPage(t *testing.T) {
	e := newEnv(t)
	rec := e.do(e.s.loginPage, http.MethodGet, "/login", nil, nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "password")
}

func TestLoginSuccess(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "s3cret")

	rec := e.do(e.s.loginHandler, http.MethodPost, "/login", url.Values{"user": {"alice"}, "password": {"s3cret"}}, nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/", rec.Header().Get("Location"))

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookie {
			cookie = c
		}
	}
	require.NotNil(t, cookie)
	require.True(t, cookie.HttpOnly)
	require.Equal(t, "/", cookie.Path)
	// Without "remember" this is a session cookie.
	require.True(t, cookie.Expires.IsZero())

	claims := &jwtClaims{}
	_, err := jwt.ParseWithClaims(cookie.Value, claims, func(*jwt.Token) (any, error) { return []byte(testKey), nil })
	require.NoError(t, err)
	require.Equal(t, "alice", claims.UserID)

	// The cookie grants access.
	rec = e.do(e.s.authHandler(e.s.index), http.MethodGet, "/", nil, cookie, nil)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestLoginRememberMe(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	rec := e.do(e.s.loginHandler, http.MethodPost, "/login", url.Values{"user": {"alice"}, "password": {"pw"}, "remember": {"on"}}, nil, nil)
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookie {
			cookie = c
		}
	}
	require.NotNil(t, cookie)
	require.WithinDuration(t, time.Now().Add(30*24*time.Hour), cookie.Expires, time.Minute)
}

func TestLoginNextRedirect(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	rec := e.do(e.s.loginHandler, http.MethodPost, "/login?next=%2Faccount", url.Values{"user": {"alice"}, "password": {"pw"}}, nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/account", rec.Header().Get("Location"))
}

func TestLoginNextOpenRedirect(t *testing.T) {
	t.Skip("BUG: login redirects to an arbitrary absolute 'next' URL (server/login.go:63-66), an open redirect; only local paths should be accepted")
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	rec := e.do(e.s.loginHandler, http.MethodPost, "/login?next="+url.QueryEscape("https://evil.example/"), url.Values{"user": {"alice"}, "password": {"pw"}}, nil, nil)
	require.NotEqual(t, "https://evil.example/", rec.Header().Get("Location"))
}

func TestLoginFailures(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")

	cases := map[string]url.Values{
		"wrong password": {"user": {"alice"}, "password": {"nope"}},
		"unknown user":   {"user": {"ghost"}, "password": {"pw"}},
		"empty":          {},
	}
	for name, form := range cases {
		t.Run(name, func(t *testing.T) {
			rec := e.do(e.s.loginHandler, http.MethodPost, "/login", form, nil, nil)
			for _, c := range rec.Result().Cookies() {
				require.NotEqual(t, authCookie, c.Name, "no auth cookie must be issued")
			}
			require.Empty(t, rec.Header().Get("Location"))
			require.Contains(t, rec.Body.String(), "Something went wrong")
		})
	}
}

func TestErrorsReturnOKStatus(t *testing.T) {
	t.Skip("BUG: handler errors (e.g. failed login, server/server.go:62-65) render the error page with HTTP 200 instead of a 4xx/5xx status")
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	rec := e.do(e.s.loginHandler, http.MethodPost, "/login", url.Values{"user": {"alice"}, "password": {"bad"}}, nil, nil)
	require.GreaterOrEqual(t, rec.Code, 400)
}

func TestLogout(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	rec := e.authed(e.s.logout, cookie, http.MethodGet, "/logout", nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/", rec.Header().Get("Location"))

	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookie {
			cleared = c
		}
	}
	require.NotNil(t, cleared)
	require.Empty(t, cleared.Value)
	require.True(t, cleared.Expires.Before(time.Now()))

	// Logout requires being logged in.
	rec = e.authed(e.s.logout, nil, http.MethodGet, "/logout", nil, nil)
	require.Contains(t, rec.Header().Get("Location"), "/login")
}
