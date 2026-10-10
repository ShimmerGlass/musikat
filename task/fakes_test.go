package task

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/musicbrainz"
	"github.com/shimmerglass/musikat/navidrome"
	"github.com/shimmerglass/musikat/subsonic"
	"github.com/stretchr/testify/require"
)

// subArtist is an artist known to the fake Subsonic server.
type subArtist struct {
	ID    string
	Name  string
	MBzID string
}

// fakeSubsonic serves the subset of the Subsonic API used by musikat.
type fakeSubsonic struct {
	*httptest.Server

	mu      sync.Mutex
	artists []subArtist
	starred []subArtist
}

func newFakeSubsonic(t *testing.T) *fakeSubsonic {
	f := &fakeSubsonic{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeSubsonic) set(artists, starred []subArtist) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.artists = artists
	f.starred = starred
}

func xmlArtist(a subArtist) string {
	return fmt.Sprintf(`<artist id=%q name=%q albumCount="1"/>`, a.ID, a.Name)
}

func (f *fakeSubsonic) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	endpoint := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	body := ""
	switch endpoint {
	case "ping":
	case "getArtists":
		body = `<artists ignoredArticles=""><index name="A">`
		for _, a := range f.artists {
			body += xmlArtist(a)
		}
		body += `</index></artists>`
	case "getStarred2":
		body = `<starred2>`
		for _, a := range f.starred {
			body += xmlArtist(a)
		}
		body += `</starred2>`
	case "search3":
		q := strings.ToLower(r.URL.Query().Get("query"))
		body = `<searchResult3>`
		for _, a := range append(append([]subArtist{}, f.artists...), f.starred...) {
			if strings.Contains(strings.ToLower(a.Name), q) {
				body += xmlArtist(a)
			}
		}
		body += `</searchResult3>`
	case "getArtistInfo2":
		id := r.URL.Query().Get("id")
		mbid := ""
		for _, a := range append(append([]subArtist{}, f.artists...), f.starred...) {
			if a.ID == id {
				mbid = a.MBzID
			}
		}
		body = fmt.Sprintf(`<artistInfo2><musicBrainzId>%s</musicBrainzId></artistInfo2>`, mbid)
	default:
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/xml")
	_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><subsonic-response xmlns="http://subsonic.org/restapi" status="ok" version="1.16.1">%s</subsonic-response>`, body)
}

// fakeNavidrome serves /auth/login and /api/song.
type fakeNavidrome struct {
	*httptest.Server

	mu    sync.Mutex
	songs map[string][]model.MediaFile // by subsonic artist ID
}

func newFakeNavidrome(t *testing.T) *fakeNavidrome {
	f := &fakeNavidrome{songs: map[string][]model.MediaFile{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/auth/login":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
		case "/api/song":
			if r.Header.Get("x-nd-authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			songs := f.songs[r.URL.Query().Get("artist_id")]
			if songs == nil {
				songs = []model.MediaFile{}
			}
			_ = json.NewEncoder(w).Encode(songs)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeNavidrome) setSongs(artistID string, songs ...model.MediaFile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.songs[artistID] = songs
}

// mbRG is a release group served by the fake MusicBrainz.
type mbRG struct {
	ID        string
	Title     string
	Primary   string
	Secondary []string
	Date      string
	Artists   []database.Artist
}

// mbRelease is a release served by the fake MusicBrainz.
type mbRelease struct {
	ID         string
	Recordings []string
}

// fakeMB serves the MusicBrainz browse endpoints for release groups and releases.
type fakeMB struct {
	*httptest.Server

	mu       sync.Mutex
	rgs      map[string][]mbRG      // by artist MBID
	releases map[string][]mbRelease // by release group MBID
	requests []string
	// pageSize, if > 0, caps the number of items per response.
	pageSize int
}

func newFakeMB(t *testing.T) *fakeMB {
	f := &fakeMB{rgs: map[string][]mbRG{}, releases: map[string][]mbRelease{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeMB) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeMB) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.String())

	q := r.URL.Query()
	offset := 0
	_, _ = fmt.Sscan(q.Get("offset"), &offset)
	page := func(n int) (int, int) {
		end := n
		if f.pageSize > 0 && offset+f.pageSize < end {
			end = offset + f.pageSize
		}
		if offset > n {
			offset = n
		}
		return offset, end
	}

	w.Header().Set("Content-Type", "application/json")
	switch strings.Trim(r.URL.Path, "/") {
	case "ws/2/release-group", "release-group":
		all := f.rgs[q.Get("artist")]
		from, to := page(len(all))
		out := []map[string]any{}
		for _, rg := range all[from:to] {
			credit := []map[string]any{}
			for _, a := range rg.Artists {
				credit = append(credit, map[string]any{"name": a.Name, "artist": map[string]any{"id": a.MBzID, "name": a.Name}})
			}
			sec := rg.Secondary
			if sec == nil {
				sec = []string{}
			}
			out = append(out, map[string]any{
				"id": rg.ID, "title": rg.Title, "primary-type": rg.Primary,
				"secondary-types": sec, "first-release-date": rg.Date,
				"artist-credit": credit,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"release-group-count": len(all), "release-group-offset": from, "release-groups": out,
		})
	case "ws/2/release", "release":
		all := f.releases[q.Get("release-group")]
		from, to := page(len(all))
		out := []map[string]any{}
		for _, rel := range all[from:to] {
			tracks := []map[string]any{}
			for _, rec := range rel.Recordings {
				tracks = append(tracks, map[string]any{"recording": map[string]any{"id": rec}})
			}
			out = append(out, map[string]any{
				"id": rel.ID, "media": []map[string]any{{"tracks": tracks}},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"release-count": len(all), "release-offset": from, "releases": out,
		})
	default:
		http.NotFound(w, r)
	}
}

// env wires the real clients to the fake servers and a real temporary DB.
type env struct {
	db   *database.DB
	sub  *fakeSubsonic
	navi *fakeNavidrome
	mb   *fakeMB

	subClient  *subsonic.Subsonic
	naviClient *navidrome.Client
	mbClient   *musicbrainz.MusicBrainz
}

func newEnv(t *testing.T) *env {
	e := &env{
		db:   newTestDB(t),
		sub:  newFakeSubsonic(t),
		navi: newFakeNavidrome(t),
		mb:   newFakeMB(t),
	}
	e.subClient = subsonic.New(subsonic.Config{URL: e.sub.URL})
	e.naviClient = navidrome.New(navidrome.Config{URL: e.navi.URL})
	e.mbClient = musicbrainz.New(
		musicbrainz.WithBaseURL(e.mb.URL),
		musicbrainz.WithCoverArtBaseURL(e.mb.URL),
		musicbrainz.WithRateLimit(time.Millisecond),
	)
	return e
}

func (e *env) addUser(t *testing.T, id string) database.User {
	u := database.User{ID: id, Name: id, SubsonicUser: id, SubsonicPass: "pw"}
	require.NoError(t, e.db.CreateUser(context.Background(), u))
	return u
}
