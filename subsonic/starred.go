package subsonic

import (
	"context"

	"github.com/shimmerglass/musikat/database"
)

func (s *User) Starred(ctx context.Context) ([]database.Artist, error) {
	starred, err := s.client.GetStarred2(map[string]string{})
	if err != nil {
		return nil, err
	}

	res := []database.Artist{}

	for _, artist := range starred.Artist {
		d, err := s.artist(ctx, artist)
		if err != nil {
			return nil, err
		}

		res = append(res, d)
	}

	return res, nil
}
