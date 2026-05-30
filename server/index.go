package server

import (
	"context"
	"net/http"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) index(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	artists, err := s.db.UserWatchedArtistsWithStats(ctx, user)
	if err != nil {
		return err
	}

	return component.Index(artists).Render(ctx, rw)
}
