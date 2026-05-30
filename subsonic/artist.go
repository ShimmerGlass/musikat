package subsonic

import (
	"context"
	"errors"
	"fmt"

	"github.com/delucks/go-subsonic"
	"github.com/shimmerglass/musikat/database"
)

var ErrArtistNotFound = fmt.Errorf("artist not found")

func (s *User) ArtistReleases(ctx context.Context, artist database.Artist) ([]string, error) {
	id, err := s.artistSubsonicID(ctx, artist)
	if errors.Is(err, ErrArtistNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	subArtist, err := s.client.GetArtist(id)
	if err != nil {
		return nil, err
	}

	res := []string{}

	for _, album := range subArtist.Album {
		info, err := s.client.GetAlbumInfo2(album.ID)
		if err != nil {
			return nil, err
		}

		res = append(res, info.MusicBrainzID)
	}

	return res, nil
}

func (s *User) artistSubsonicID(ctx context.Context, artist database.Artist) (string, error) {
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

		if artistInfo.MBzID == artist.MBzID {
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
