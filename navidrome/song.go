package navidrome

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/navidrome/navidrome/model"
)

func (u *User) ArtistSongs(ctx context.Context, artist string) ([]model.MediaFile, error) {
	q := url.Values{}
	q.Set("artist_id", artist)
	ur, err := url.JoinPath(u.config.URL, "/api/song")
	if err != nil {
		return nil, fmt.Errorf("navidrome: artist songs: %w", err)
	}
	ur += "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ur, nil)
	if err != nil {
		return nil, fmt.Errorf("navidrome: artist songs: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-nd-authorization", "Bearer "+u.token)

	res, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("navidrome: artist songs: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("navidrome: artist songs: invalid status code %d", res.StatusCode)
	}

	songs := []model.MediaFile{}
	err = json.NewDecoder(res.Body).Decode(&songs)
	if err != nil {
		return nil, fmt.Errorf("navidrome: artist songs: %w", err)
	}

	return songs, nil
}
