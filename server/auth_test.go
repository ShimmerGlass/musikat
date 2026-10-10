package server

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func signToken(t *testing.T, method jwt.SigningMethod, key any, uid string, exp time.Time) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, jwtClaims{
		UserID:           uid,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(exp)},
	})
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

func TestAuthRedirectsWithoutUsersToSetup(t *testing.T) {
	e := newEnv(t)
	rec := e.do(e.s.authHandler(e.s.index), http.MethodGet, "/", nil, nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/setup", rec.Header().Get("Location"))
}

func TestAuthRedirectsToLoginWithNext(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	rec := e.do(e.s.authHandler(e.s.index), http.MethodGet, "/artists/abc?x=1&y=2", nil, nil, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Equal(t, "/login?next="+url.QueryEscape("/artists/abc?x=1&y=2"), rec.Header().Get("Location"))
}

func TestAuthInvalidCookies(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	future := time.Now().Add(time.Hour)

	cases := map[string]string{
		"empty":        "",
		"garbage":      "not-a-jwt",
		"wrong key":    signToken(t, jwt.SigningMethodHS256, []byte("other"), "alice", future),
		"expired":      signToken(t, jwt.SigningMethodHS256, []byte(testKey), "alice", time.Now().Add(-time.Hour)),
		"wrong alg":    signToken(t, jwt.SigningMethodHS512, []byte(testKey), "alice", future),
		"unknown user": signToken(t, jwt.SigningMethodHS256, []byte(testKey), "ghost", future),
		"none alg-ish": "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ1aWQiOiJhbGljZSJ9.",
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			rec := e.do(e.s.authHandler(e.s.index), http.MethodGet, "/", nil, &http.Cookie{Name: authCookie, Value: val}, nil)
			require.Equal(t, http.StatusSeeOther, rec.Code)
			require.Contains(t, rec.Header().Get("Location"), "/login")
		})
	}
}

func TestAuthValidCookieAllowsAccess(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	c := signToken(t, jwt.SigningMethodHS256, []byte(testKey), "alice", time.Now().Add(time.Hour))
	rec := e.do(e.s.authHandler(e.s.index), http.MethodGet, "/", nil, &http.Cookie{Name: authCookie, Value: c}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthDeletedUserLosesAccess(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	e.addUser("bob", "Bob", "pw")
	cookie := e.login("bob", "pw")
	require.NoError(t, e.db.UserDelete(e.ctx, "bob"))
	rec := e.do(e.s.authHandler(e.s.index), http.MethodGet, "/", nil, cookie, nil)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "/login")
}
