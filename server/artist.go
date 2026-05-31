package server

import (
	"context"
	"net/http"
	"path"

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

	watch, _, err := s.db.ArtistWatch(ctx, user.ID, artistID)
	if err != nil {
		return err
	}

	rgs, err := s.db.ArtistReleaseGroups(ctx, artist)
	if err != nil {
		return err
	}

	return component.Artist(artists, artist, watch, rgs).Render(ctx, rw)
}

func (s *Server) artistWatch(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	artistID := r.PathValue("mbid")
	watch, _, err := s.db.ArtistWatch(ctx, user.ID, artistID)
	if err != nil {
		return err
	}

	watch.Status = true
	if watch.Source == "" {
		watch.Source = "ui"
	}

	err = s.db.AddArtistWatch(ctx, watch)
	if err != nil {
		return err
	}

	http.Redirect(rw, r, path.Join("/artists", artistID), http.StatusSeeOther)
	return nil
}

func (s *Server) artistStopWatch(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	artistID := r.PathValue("mbid")
	watch, _, err := s.db.ArtistWatch(ctx, user.ID, artistID)
	if err != nil {
		return err
	}

	watch.Status = false

	err = s.db.AddArtistWatch(ctx, watch)
	if err != nil {
		return err
	}

	http.Redirect(rw, r, path.Join("/artists", artistID), http.StatusSeeOther)
	return nil
}
