package server

import (
	"context"
	"net/http"

	"github.com/shimmerglass/musikat/database"
)

func (s *Server) refresh(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	query := r.URL.Query()

	if artistID := query.Get("artist"); artistID != "" {
		artist, err := s.db.Artist(ctx, artistID)
		if err != nil {
			return err
		}

		s.tasks.RefreshArtist(artist)
	} else {
		s.tasks.Refresh()
	}

	redirect := "/"
	if next := query.Get("next"); next != "" {
		redirect = next
	}

	http.Redirect(rw, r, redirect, http.StatusSeeOther)
	return nil
}
