package server

import (
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) artistWatch(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
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

	return component.WatchEdit(artists, watch, artist).Render(ctx, rw)
}

func (s *Server) artistWatchUpdate(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	artistID := r.PathValue("mbid")

	watch, _, err := s.db.ArtistWatch(ctx, user.ID, artistID)
	if err != nil {
		return err
	}

	watch.XXPrimaryTypes = strings.Join(r.Form["primary_types[]"], ",")
	watch.XXSecondaryTypes = strings.Join(r.Form["secondary_types[]"], ",")

	err = s.db.AddArtistWatch(ctx, watch)
	if err != nil {
		return err
	}

	http.Redirect(rw, r, path.Join("/artists", artistID), http.StatusSeeOther)
	return nil
}
