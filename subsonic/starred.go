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

	return s.artists(ctx, starred.Artist)
}
