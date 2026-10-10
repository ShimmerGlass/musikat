package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

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
	require.NoError(e.t, e.db.CreateUser(e.ctx, u))
	return u
}

func (e *env) addAdmin(id, name, pass string) database.User {
	e.t.Helper()
	hash, err := auth.HashPass(pass)
	require.NoError(e.t, err)
	u := database.User{ID: id, Name: name, Password: hash, Admin: true}
	require.NoError(e.t, e.db.CreateUser(e.ctx, u))
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

// adminAuthed is like authed but also applies the admin check, as the real
// admin routes do.
func (e *env) adminAuthed(h userHandler, cookie *http.Cookie, method, target string, form url.Values, pathVals map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(e.s.authHandler(e.s.adminHandler(h)), method, target, form, cookie, pathVals)
}

func TestRoutingAndMethods(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")

	srv := httptest.NewServer(e.s.Handler())
	t.Cleanup(srv.Close)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := srv.URL

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
	for _, p := range []string{"/", "/account", "/admin/users", "/admin/users/create", "/artists/x", "/artist-watches/x"} {
		resp := get("GET", p)
		require.Equal(t, http.StatusSeeOther, resp.StatusCode, p)
		require.Equal(t, "/login?next="+url.QueryEscape(p), resp.Header.Get("Location"), p)
	}

	// State-changing routes are POST only.
	for _, p := range []string{"/refresh", "/logout", "/artists/x/watch", "/artists/x/stop-watch"} {
		require.Equal(t, http.StatusMethodNotAllowed, get("GET", p).StatusCode, p)
		resp := get("POST", p)
		require.Equal(t, http.StatusSeeOther, resp.StatusCode, p)
		require.Contains(t, resp.Header.Get("Location"), "/login", p)
	}

	// Wrong methods.
	require.Equal(t, http.StatusMethodNotAllowed, get("POST", "/").StatusCode)
	require.Equal(t, http.StatusMethodNotAllowed, get("DELETE", "/login").StatusCode)

	// Setup is disabled (redirect) once a user exists.
	resp := get("GET", "/setup")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "/", resp.Header.Get("Location"))

	require.Equal(t, http.StatusNotFound, get("GET", "/nope").StatusCode)
}

func TestCrossOriginProtection(t *testing.T) {
	e := newEnv(t)
	e.addAdmin("alice", "Alice", "pw")
	e.addArtist("a1", "Radiohead")
	cookie := e.login("alice", "pw")

	srv := httptest.NewServer(e.s.Handler())
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	post := func(path string, form url.Values, headers map[string]string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(form.Encode()))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	cross := map[string]string{"Sec-Fetch-Site": "cross-site"}
	evilOrigin := map[string]string{"Origin": "https://evil.example"}

	for name, h := range map[string]map[string]string{"cross-site": cross, "evil origin": evilOrigin} {
		// Watch is not created.
		resp := post("/artists/a1/watch", nil, h)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, name)
		status, _ := e.watchStatus("alice", "a1")
		require.False(t, status, name)

		// User is not created.
		resp = post("/admin/users/create", url.Values{"login": {"eve"}, "name": {"Eve"}, "password": {"x"}}, h)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, name)
		_, err := e.db.User(e.ctx, "eve")
		require.ErrorIs(t, err, database.ErrUserNotFound, name)

		// Password is not changed.
		resp = post("/account", url.Values{"type": {"password"}, "current_pass": {"pw"}, "new_pass": {"new"}, "confirm_pass": {"new"}}, h)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, name)
		u, err := e.db.User(e.ctx, "alice")
		require.NoError(t, err)
		ok, err := auth.ComparePasswordAndHash("pw", u.Password)
		require.NoError(t, err)
		require.True(t, ok, name)

		// Logout is blocked too (cookie not cleared).
		resp = post("/logout", nil, h)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, name)
		require.Empty(t, resp.Cookies(), name)
	}

	// Same-origin requests work.
	resp := post("/artists/a1/watch", nil, map[string]string{"Sec-Fetch-Site": "same-origin"})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	status, _ := e.watchStatus("alice", "a1")
	require.True(t, status)

	// Origin matching the host is allowed when Sec-Fetch-Site is absent.
	resp = post("/artists/a1/stop-watch", nil, map[string]string{"Origin": srv.URL})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	status, _ = e.watchStatus("alice", "a1")
	require.False(t, status)

	// Non-browser clients (neither Sec-Fetch-Site nor Origin) are allowed by
	// the stdlib CrossOriginProtection.
	resp = post("/artists/a1/watch", nil, nil)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	status, _ = e.watchStatus("alice", "a1")
	require.True(t, status)
}

func TestLogoutViaRouterClearsCookie(t *testing.T) {
	e := newEnv(t)
	e.addUser("alice", "Alice", "pw")
	cookie := e.login("alice", "pw")

	srv := httptest.NewServer(e.s.Handler())
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/logout", nil)
	require.NoError(t, err)
	req.AddCookie(cookie)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	var cleared *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == authCookie {
			cleared = c
		}
	}
	require.NotNil(t, cleared)
	require.Empty(t, cleared.Value)
}
