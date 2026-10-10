package navidrome

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserLogin(t *testing.T) {
	var gotReq authRequest
	var method, path, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, ctype = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(`{"token":"tok123"}`))
	}))
	defer srv.Close()

	u, err := New(Config{URL: srv.URL}).User(context.Background(), "bob", "pw")
	require.NoError(t, err)
	require.Equal(t, "tok123", u.token)
	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "/auth/login", path)
	require.Equal(t, "application/json", ctype)
	require.Equal(t, authRequest{Username: "bob", Password: "pw"}, gotReq)
}

func TestUserLoginErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"unauthorized": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		"bad json":     func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("nope")) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			u, err := New(Config{URL: srv.URL}).User(context.Background(), "bob", "pw")
			require.Error(t, err)
			require.Nil(t, u)
		})
	}

	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		_, err := New(Config{URL: url}).User(context.Background(), "bob", "pw")
		require.Error(t, err)
	})
}

func TestArtistSongs(t *testing.T) {
	var path, auth, accept, artist string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("x-nd-authorization")
		accept = r.Header.Get("Accept")
		artist = r.URL.Query().Get("artist_id")
		_, _ = w.Write([]byte(`[{"id":"s1","title":"One","mbzRecordingID":"rec1"},{"id":"s2","title":"Two"}]`))
	}))
	defer srv.Close()

	u := &User{config: Config{URL: srv.URL}, httpClient: srv.Client(), token: "tok"}
	songs, err := u.ArtistSongs(context.Background(), "a&b=1")
	require.NoError(t, err)
	require.Len(t, songs, 2)
	require.Equal(t, "s1", songs[0].ID)
	require.Equal(t, "One", songs[0].Title)
	require.Equal(t, "rec1", songs[0].MbzRecordingID)
	require.Equal(t, "s2", songs[1].ID)

	require.Equal(t, "/api/song", path)
	require.Equal(t, "Bearer tok", auth)
	require.Equal(t, "application/json", accept)
	require.Equal(t, "a&b=1", artist, "artist id must be query-escaped")
}

func TestArtistSongsErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"status":   func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) },
		"bad json": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{")) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			u := &User{config: Config{URL: srv.URL}, httpClient: srv.Client()}
			_, err := u.ArtistSongs(context.Background(), "a")
			require.Error(t, err)
		})
	}

	t.Run("canceled context", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		u := &User{config: Config{URL: srv.URL}, httpClient: srv.Client()}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := u.ArtistSongs(ctx, "a")
		require.Error(t, err)
	})
}

func TestArtistSongsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	u := &User{config: Config{URL: srv.URL}, httpClient: srv.Client()}
	songs, err := u.ArtistSongs(context.Background(), "a")
	require.NoError(t, err)
	require.Empty(t, songs)
}
