package subsonic

import (
	"context"
	"fmt"

	"github.com/delucks/go-subsonic"
	"github.com/shimmerglass/musikat/database"
)

var ErrArtistNotFound = fmt.Errorf("artist not found")

func (s *User) ArtistSubsonicID(ctx context.Context, artist database.Artist) (string, error) {
	if artist.MBzID == "" {
		return "", ErrArtistNotFound
	}

	search, err := s.client.Search3(artist.Name, map[string]string{
		"albumCount": "0",
		"songCount":  "0",
	})
	if err != nil {
		return "", err
	}

	for _, searchArtist := range search.Artist {
		artistInfo, err := s.artist(ctx, searchArtist)
		if err != nil {
			return "", err
		}

		if artistInfo.MBzID != "" && artistInfo.MBzID == artist.MBzID {
			return searchArtist.ID, nil
		}
	}

	return "", ErrArtistNotFound
}

func (s *User) artist(ctx context.Context, in *subsonic.ArtistID3) (database.Artist, error) {
	info, err := s.client.GetArtistInfo2(in.ID, map[string]string{})
	if err != nil {
		return database.Artist{}, err
	}

	return database.Artist{
		Name:  in.Name,
		MBzID: info.MusicBrainzID,
	}, nil
}

func (s *User) Artists(ctx context.Context) ([]database.Artist, error) {
	artists, err := s.client.GetArtists(map[string]string{})
	if err != nil {
		return nil, err
	}

	var all []*subsonic.ArtistID3
	for _, index := range artists.Index {
		all = append(all, index.Artist...)
	}

	return s.artists(ctx, all)
}

func (s *User) artists(ctx context.Context, in []*subsonic.ArtistID3) ([]database.Artist, error) {
	return mapConcurrent(in, maxConcurrency, func(a *subsonic.ArtistID3) (database.Artist, error) {
		return s.artist(ctx, a)
	})
}
