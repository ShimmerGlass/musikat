package server

import (
	"context"
	"net/http"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) index(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	watchedArtists, err := s.db.UserWatchedArtistsWithStats(ctx, user)
	if err != nil {
		return err
	}

	stats, err := s.db.Stats(ctx)
	if err != nil {
		return err
	}

	artists, err := s.db.Artists(ctx)
	if err != nil {
		return err
	}

	return component.Index(watchedArtists, artists, stats).Render(ctx, rw)
}
