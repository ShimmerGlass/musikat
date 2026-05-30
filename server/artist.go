package server

import (
	"context"
	"net/http"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) artist(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	artists, err := s.db.UserWatchedArtistsWithStats(ctx, user)
	if err != nil {
		return err
	}

	artistID := r.PathValue("mbid")
	artist, err := s.db.Artist(ctx, artistID)
	if err != nil {
		return err
	}

	rgs, err := s.db.ArtistReleaseGroups(ctx, artist)
	if err != nil {
		return err
	}

	return component.Artist(artists, artist, rgs).Render(ctx, rw)
}
