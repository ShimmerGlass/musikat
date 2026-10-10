package musicbrainz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	mbz "go.uploadedlobster.com/musicbrainzws2"
)

type recordedReq struct {
	path  string
	query map[string][]string
}

// newTestMBz returns a MusicBrainz client talking to handler, with rate
// limiting disabled.
func newTestMBz(t *testing.T, handler http.HandlerFunc) (*MusicBrainz, func() []recordedReq) {
	t.Helper()

	var (
		mu   sync.Mutex
		reqs []recordedReq
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, recordedReq{path: r.URL.Path, query: r.URL.Query()})
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New()
	m.client.SetBaseURL(srv.URL)

	tick := make(chan time.Time)
	close(tick)
	m.tick = tick

	return m, func() []recordedReq {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedReq(nil), reqs...)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(v))
}

func rgJSON(id, title, primary string, secondary []string, artistID, artistName string) map[string]any {
	return map[string]any{
		"id":                 id,
		"title":              title,
		"primary-type":       primary,
		"secondary-types":    secondary,
		"first-release-date": "2001-05-06",
		"artist-credit": []map[string]any{
			{"name": artistName, "artist": map[string]any{"id": artistID, "name": artistName}},
		},
	}
}

func TestArtistReleaseGroupsMapping(t *testing.T) {
	m, reqs := newTestMBz(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"release-group-count":  3,
			"release-group-offset": 0,
			"release-groups": []any{
				rgJSON("rg1", "Album One", "Album", []string{"Live", "Remix"}, "a1", "Artist One"),
				rgJSON("rg2", "Hits", "Album", []string{"Compilation"}, "a1", "Artist One"),
				map[string]any{
					"id": "rg3", "title": "Duo Single", "primary-type": "Single",
					"first-release-date": "1999",
					"artist-credit": []any{
						map[string]any{"name": "A", "joinphrase": " & ", "artist": map[string]any{"id": "a1", "name": "Artist One"}},
						map[string]any{"name": "B", "artist": map[string]any{"id": "a2", "name": "Artist Two"}},
					},
				},
			},
		})
	})

	got, err := m.ArtistReleaseGroups(context.Background(), "a1")
	require.NoError(t, err)
	require.Len(t, got, 2, "compilation must be filtered out")

	require.Equal(t, "rg1", got[0].MBzID)
	require.Equal(t, "Album One", got[0].Name)
	require.Equal(t, "Album", got[0].PrimaryType)
	require.Equal(t, "Live,Remix", got[0].XXSecondaryTypes)
	require.Equal(t, "2001-05-06", got[0].ReleaseDate)
	require.Len(t, got[0].Artists, 1)
	require.Equal(t, "a1", got[0].Artists[0].MBzID)
	require.Equal(t, "Artist One", got[0].Artists[0].Name)

	require.Equal(t, "rg3", got[1].MBzID)
	require.Equal(t, "Single", got[1].PrimaryType)
	require.Empty(t, got[1].XXSecondaryTypes)
	require.Equal(t, "1999", got[1].ReleaseDate)
	require.Len(t, got[1].Artists, 2)
	require.Equal(t, "a2", got[1].Artists[1].MBzID)
	require.Equal(t, "Artist Two", got[1].Artists[1].Name)

	rs := reqs()
	require.Len(t, rs, 1)
	require.Equal(t, "/release-group/", rs[0].path)
	require.Equal(t, []string{"a1"}, rs[0].query["artist"])
	require.Equal(t, []string{"artist-credits"}, rs[0].query["inc"])
	require.Equal(t, []string{strconv.Itoa(mbz.MaxLimit)}, rs[0].query["limit"])
	require.Equal(t, []string{"0"}, rs[0].query["offset"])
}

func TestArtistReleaseGroupsPagination(t *testing.T) {
	m, reqs := newTestMBz(t, func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		all := []string{"rg0", "rg1", "rg2", "rg3", "rg4"}
		end := min(offset+2, len(all))
		var page []any
		for _, id := range all[offset:end] {
			page = append(page, rgJSON(id, id, "Album", nil, "a1", "A"))
		}
		writeJSON(t, w, map[string]any{
			"release-group-count":  len(all),
			"release-group-offset": offset,
			"release-groups":       page,
		})
	})

	got, err := m.ArtistReleaseGroups(context.Background(), "a1")
	require.NoError(t, err)
	var ids []string
	for _, rg := range got {
		ids = append(ids, rg.MBzID)
	}
	require.Equal(t, []string{"rg0", "rg1", "rg2", "rg3", "rg4"}, ids)

	rs := reqs()
	require.Len(t, rs, 3)
	for i, off := range []string{"0", "2", "4"} {
		require.Equal(t, []string{off}, rs[i].query["offset"])
	}
}

func TestArtistReleaseGroupsEmpty(t *testing.T) {
	m, reqs := newTestMBz(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"release-group-count": 0, "release-groups": []any{}})
	})
	got, err := m.ArtistReleaseGroups(context.Background(), "a1")
	require.NoError(t, err)
	require.Empty(t, got)
	require.Len(t, reqs(), 1)
}

func TestArtistReleaseGroupsError(t *testing.T) {
	m, _ := newTestMBz(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(t, w, map[string]any{"error": "bad"})
	})
	_, err := m.ArtistReleaseGroups(context.Background(), "a1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "mbz artist release groups")
}

func TestReleaseGroupsReleases(t *testing.T) {
	m, reqs := newTestMBz(t, func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		all := []string{"r0", "r1", "r2"}
		end := min(offset+2, len(all))
		var page []any
		for _, id := range all[offset:end] {
			page = append(page, map[string]any{"id": id, "title": "T " + id})
		}
		writeJSON(t, w, map[string]any{
			"release-count":  len(all),
			"release-offset": offset,
			"releases":       page,
		})
	})

	got, err := m.ReleaseGroupsReleases(context.Background(), "rg1")
	require.NoError(t, err)
	require.Len(t, got, 3)
	for i, r := range got {
		require.EqualValues(t, "r"+strconv.Itoa(i), r.ID)
	}

	rs := reqs()
	require.Len(t, rs, 2)
	require.Equal(t, "/release/", rs[0].path)
	require.Equal(t, []string{"rg1"}, rs[0].query["release-group"])
	require.Equal(t, []string{"recordings"}, rs[0].query["inc"])
	require.Equal(t, []string{"2"}, rs[1].query["offset"])
}

func TestReleaseGroupsReleasesError(t *testing.T) {
	m, _ := newTestMBz(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(t, w, map[string]any{"error": "bad"})
	})
	_, err := m.ReleaseGroupsReleases(context.Background(), "rg1")
	require.Error(t, err)
}
