package musicbrainz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseGroupCoverURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		want    bool
		wantErr bool
	}{
		{"redirect", http.StatusTemporaryRedirect, true, false},
		{"ok", http.StatusOK, true, false},
		{"not found", http.StatusNotFound, false, false},
		{"server error", http.StatusInternalServerError, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var method, path string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				if tc.status == http.StatusTemporaryRedirect {
					w.Header().Set("Location", "/elsewhere")
				}
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			m := New()
			m.coverArtBaseURL = srv.URL

			got, err := m.ReleaseGroupCoverURL(context.Background(), "abc")
			require.Equal(t, http.MethodHead, method)
			require.Equal(t, "/release-group/abc/front-500", path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.want {
				require.Equal(t, srv.URL+"/release-group/abc/front-500", got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}
