package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/shimmerglass/musikat/database"
)

func (s *Server) refresh(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	query := r.URL.Query()

	if artistID := query.Get("artist"); artistID != "" {
		artist, err := s.db.Artist(ctx, artistID)
		if errors.Is(err, database.ErrArtistNotFound) {
			return notFound(err)
		}
		if err != nil {
			return err
		}

		s.tasks.RefreshArtist(artist)
	} else {
		s.tasks.Refresh()
	}

	http.Redirect(rw, r, localRedirect(query.Get("next")), http.StatusSeeOther)
	return nil
}
