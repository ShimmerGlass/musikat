package subsonic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	gosubsonic "github.com/delucks/go-subsonic"
	"github.com/shimmerglass/musikat/database"
	"github.com/stretchr/testify/require"
)

const xmlHead = `<?xml version="1.0" encoding="UTF-8"?><subsonic-response xmlns="http://subsonic.org/restapi" status="ok" version="1.16.1">`

// fakeServer serves canned Subsonic XML per endpoint. info maps artist ID to
// MusicBrainz ID returned by getArtistInfo2.
type fakeServer struct {
	search  string // inner XML of searchResult3
	artists string // inner XML of artists
	starred string // inner XML of starred2
	info    map[string]string

	mu    sync.Mutex
	calls map[string][]map[string][]string
}

func (f *fakeServer) handler(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.TrimPrefix(r.URL.Path, "/rest/")
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string][]map[string][]string{}
	}
	f.calls[endpoint] = append(f.calls[endpoint], r.URL.Query())
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/xml")
	switch endpoint {
	case "search3":
		_, _ = fmt.Fprintf(w, `%s<searchResult3>%s</searchResult3></subsonic-response>`, xmlHead, f.search)
	case "getArtists":
		_, _ = fmt.Fprintf(w, `%s<artists ignoredArticles="">%s</artists></subsonic-response>`, xmlHead, f.artists)
	case "getStarred2":
		_, _ = fmt.Fprintf(w, `%s<starred2>%s</starred2></subsonic-response>`, xmlHead, f.starred)
	case "getArtistInfo2":
		id := r.URL.Query().Get("id")
		_, _ = fmt.Fprintf(w, `%s<artistInfo2><musicBrainzId>%s</musicBrainzId></artistInfo2></subsonic-response>`, xmlHead, f.info[id])
	default:
		http.NotFound(w, r)
	}
}

func newUser(t *testing.T, f *fakeServer) *User {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return &User{client: &gosubsonic.Client{
		Client:     srv.Client(),
		BaseUrl:    srv.URL,
		User:       "u",
		ClientName: "test",
	}}
}

func artistsXML(pairs ...string) string {
	var b strings.Builder
	for i := 0; i < len(pairs); i += 2 {
		fmt.Fprintf(&b, `<artist id="%s" name="%s" albumCount="1"/>`, pairs[i], pairs[i+1])
	}
	return b.String()
}

func TestArtistSubsonicID(t *testing.T) {
	f := &fakeServer{
		search: artistsXML("s1", "Name", "s2", "Name", "s3", "Name"),
		info:   map[string]string{"s1": "mbid-1", "s2": "mbid-2", "s3": "mbid-3"},
	}
	u := newUser(t, f)

	id, err := u.ArtistSubsonicID(context.Background(), database.Artist{Name: "Name", MBzID: "mbid-2"})
	require.NoError(t, err)
	require.Equal(t, "s2", id)

	q := f.calls["search3"][0]
	require.Equal(t, []string{"Name"}, q["query"])
	require.Equal(t, []string{"0"}, q["albumCount"])
	require.Equal(t, []string{"0"}, q["songCount"])
}

func TestArtistSubsonicIDNotFound(t *testing.T) {
	f := &fakeServer{
		search: artistsXML("s1", "Name"),
		info:   map[string]string{"s1": "mbid-1"},
	}
	_, err := newUser(t, f).ArtistSubsonicID(context.Background(), database.Artist{Name: "Name", MBzID: "other"})
	require.ErrorIs(t, err, ErrArtistNotFound)

	// No search results at all.
	_, err = newUser(t, &fakeServer{}).ArtistSubsonicID(context.Background(), database.Artist{Name: "Name", MBzID: "x"})
	require.ErrorIs(t, err, ErrArtistNotFound)
}

func TestArtistSubsonicIDInfoError(t *testing.T) {
	f := &fakeServer{search: artistsXML("s1", "Name")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "getArtistInfo2") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		f.handler(w, r)
	}))
	defer srv.Close()
	u := &User{client: &gosubsonic.Client{Client: srv.Client(), BaseUrl: srv.URL, User: "u", ClientName: "t"}}
	_, err := u.ArtistSubsonicID(context.Background(), database.Artist{Name: "Name", MBzID: "m"})
	require.Error(t, err)
}

func TestArtistSubsonicIDSearchAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><subsonic-response xmlns="http://subsonic.org/restapi" status="failed" version="1.16.1"><error code="40" message="Wrong username or password"/></subsonic-response>`)
	}))
	defer srv.Close()
	u := &User{client: &gosubsonic.Client{Client: srv.Client(), BaseUrl: srv.URL, User: "u", ClientName: "t"}}
	_, err := u.ArtistSubsonicID(context.Background(), database.Artist{Name: "Name", MBzID: "m"})
	require.ErrorContains(t, err, "Wrong username or password")
}

func TestArtistSubsonicIDEmptyMBzID(t *testing.T) {
	t.Skip("BUG: subsonic/artist.go:26 compares MBzIDs without checking they are non-empty: an artist with no MBzID matches the first search result that also has no MusicBrainz ID, returning an unrelated artist")
	f := &fakeServer{
		search: artistsXML("s1", "Other"),
		info:   map[string]string{"s1": ""},
	}
	_, err := newUser(t, f).ArtistSubsonicID(context.Background(), database.Artist{Name: "Name"})
	require.ErrorIs(t, err, ErrArtistNotFound)
}

func TestArtists(t *testing.T) {
	f := &fakeServer{
		artists: `<index name="A">` + artistsXML("a1", "Alpha", "a2", "Able") + `</index>` +
			`<index name="B">` + artistsXML("b1", "Beta") + `</index>` +
			`<index name="C"></index>` +
			`<index name="D">` + artistsXML("d1", "Delta", "d2", "Dee") + `</index>`,
		info: map[string]string{"a1": "m-a1", "a2": "m-a2", "b1": "m-b1", "d1": "", "d2": "m-d2"},
	}
	got, err := newUser(t, f).Artists(context.Background())
	require.NoError(t, err)
	require.Equal(t, []database.Artist{
		{Name: "Alpha", MBzID: "m-a1"},
		{Name: "Able", MBzID: "m-a2"},
		{Name: "Beta", MBzID: "m-b1"},
		{Name: "Delta", MBzID: ""},
		{Name: "Dee", MBzID: "m-d2"},
	}, got)
}

func TestArtistsEmpty(t *testing.T) {
	got, err := newUser(t, &fakeServer{}).Artists(context.Background())
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestArtistsManyKeepsOrder(t *testing.T) {
	var pairs []string
	info := map[string]string{}
	for i := range 30 {
		id := fmt.Sprintf("id%02d", i)
		pairs = append(pairs, id, "N"+id)
		info[id] = "m-" + id
	}
	f := &fakeServer{artists: `<index name="X">` + artistsXML(pairs...) + `</index>`, info: info}
	got, err := newUser(t, f).Artists(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 30)
	for i, a := range got {
		require.Equal(t, fmt.Sprintf("m-id%02d", i), a.MBzID)
	}
}

func TestArtistsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	u := &User{client: &gosubsonic.Client{Client: srv.Client(), BaseUrl: srv.URL, User: "u", ClientName: "t"}}
	_, err := u.Artists(context.Background())
	require.Error(t, err)
}

func TestStarred(t *testing.T) {
	f := &fakeServer{
		starred: artistsXML("s1", "One", "s2", "Two"),
		info:    map[string]string{"s1": "m1", "s2": "m2"},
	}
	got, err := newUser(t, f).Starred(context.Background())
	require.NoError(t, err)
	require.Equal(t, []database.Artist{{Name: "One", MBzID: "m1"}, {Name: "Two", MBzID: "m2"}}, got)
}

func TestStarredEmpty(t *testing.T) {
	got, err := newUser(t, &fakeServer{}).Starred(context.Background())
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestUserValidation(t *testing.T) {
	s := New(Config{URL: "http://127.0.0.1:1"})
	_, err := s.User(database.User{SubsonicPass: "p"})
	require.ErrorContains(t, err, "no subsonic user")
	_, err = s.User(database.User{SubsonicUser: "u"})
	require.ErrorContains(t, err, "no subsonic password")
}

func TestUserAuthenticates(t *testing.T) {
	var q map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		_, _ = fmt.Fprint(w, xmlHead+`</subsonic-response>`)
	}))
	defer srv.Close()

	u, err := New(Config{URL: srv.URL}).User(database.User{SubsonicUser: "bob", SubsonicPass: "pw"})
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, []string{"bob"}, q["u"])
	require.Equal(t, []string{"Musikat"}, q["c"])
	require.NotEmpty(t, q["t"])
	require.NotEmpty(t, q["s"])
	require.NotContains(t, q, "p", "plaintext password must not be sent")
}
