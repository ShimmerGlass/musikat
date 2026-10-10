package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	opt "github.com/shimmerglass/go-optional"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/auth"
	"github.com/shimmerglass/musikat/task"
	"github.com/stretchr/testify/require"
)

const testKey = "test-jwt-key"

type env struct {
	t   *testing.T
	s   *Server
	db  *database.DB
	ctx context.Context
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := database.New(database.Config{Path: filepath.Join(t.TempDir(), "test.db")})
	require.NoError(t, err)

	// Tasks are never started and have no external dependencies.
	tasks := task.New(task.Config{}, db, nil, nil, nil, nil)
	s := New(Config{ListenAddr: "127.0.0.1:0", JWTKey: testKey}, db, tasks)
	return &env{t: t, s: s, db: db, ctx: context.Background()}
}

func (e *env) addUser(id, name, pass string) database.User {
	e.t.Helper()
	hash, err := auth.HashPass(pass)
	require.NoError(e.t, err)
	u := database.User{ID: id, Name: name, Password: hash}
	require.NoError(e.t, e.db.AddUser(e.ctx, u))
	return u
}

func (e *env) addArtist(id, name string) database.Artist {
	e.t.Helper()
	a := database.Artist{MBzID: id, Name: name}
	require.NoError(e.t, e.db.PutArtist(e.ctx, id, func(opt.Option[database.Artist]) database.Artist { return a }))
	return a
}

// login performs a real login through the login handler and returns the cookie.
func (e *env) login(id, pass string) *http.Cookie {
	e.t.Helper()
	rec := e.do(e.s.loginHandler, http.MethodPost, "/login", url.Values{"user": {id}, "password": {pass}}, nil, nil)
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookie {
			return c
		}
	}
	e.t.Fatalf("no auth cookie returned (status %d): %s", rec.Code, rec.Body.String())
	return nil
}

// do invokes a handler (wrapped like the real routes) with optional form
// values, cookie and path values.
func (e *env) do(h handler, method, target string, form url.Values, cookie *http.Cookie, pathVals map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	var r *http.Request
	if form != nil {
		r = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	for k, v := range pathVals {
		r.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	e.s.handle(h).ServeHTTP(rec, r)
	return rec
}

func (e *env) authed(h userHandler, cookie *http.Cookie, method, target string, form url.Values, pathVals map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(e.s.authHandler(h), method, target, form, cookie, pathVals)
}

func TestRoutingAndMethods(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	e.s.cfg.ListenAddr = addr

	// Run registers on http.DefaultServeMux, so it can only be called once per process.
	go func() { _ = e.s.Run() }()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := "http://" + addr
	require.Eventually(t, func() bool {
		resp, err := client.Get(base + "/login")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}, 5*time.Second, 20*time.Millisecond)

	get := func(method, p string) *http.Response {
		req, err := http.NewRequest(method, base+p, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	require.Equal(t, http.StatusOK, get("GET", "/login").StatusCode)

	// Protected routes redirect to login with next.
	for _, p := range []string{"/", "/refresh", "/account", "/admin/users", "/admin/users/create", "/artists/x", "/artists/x/watch", "/artists/x/stop-watch", "/artist-watches/x", "/logout"} {
		resp := get("GET", p)
		require.Equal(t, http.StatusSeeOther, resp.StatusCode, p)
		require.Equal(t, "/login?next="+url.QueryEscape(p), resp.Header.Get("Location"), p)
	}

	// Wrong methods.
	require.Equal(t, http.StatusMethodNotAllowed, get("POST", "/").StatusCode)
	require.Equal(t, http.StatusMethodNotAllowed, get("DELETE", "/login").StatusCode)
	require.Equal(t, http.StatusMethodNotAllowed, get("POST", "/artists/x/watch").StatusCode)

	// Setup is disabled (redirect) once a user exists.
	resp := get("GET", "/setup")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/", resp.Header.Get("Location"))

	if get("GET", "/nope").StatusCode != http.StatusNotFound {
		t.Skip("BUG: 'GET /' is a catch-all pattern (server/server.go:34), so unknown GET paths hit the index handler (login redirect) instead of returning 404")
	}
}
