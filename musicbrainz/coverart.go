package musicbrainz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const defaultCoverArtBaseURL = "https://coverartarchive.org"

const userAgent = "Musikat/0.0.1 ( https://github.com/ShimmerGlass/musikat )"

func newCoverArtClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		// Do not follow redirects: a redirect already proves the image exists.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ReleaseGroupCoverURL returns the Cover Art Archive front cover URL for a
// release group, or "" if there is none.
func (m *MusicBrainz) ReleaseGroupCoverURL(ctx context.Context, rgID string) (string, error) {
	imageURL := m.coverArtBaseURL + "/release-group/" + url.PathEscape(rgID) + "/front-500"

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("cover art request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	res, err := m.coverArtClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cover art request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	switch {
	case res.StatusCode >= 200 && res.StatusCode < 400:
		return imageURL, nil
	case res.StatusCode == http.StatusNotFound:
		return "", nil
	default:
		return "", fmt.Errorf("cover art: unexpected status %d", res.StatusCode)
	}
}
