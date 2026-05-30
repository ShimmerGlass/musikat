package server

import (
	"context"
	"net/http"

	"github.com/shimmerglass/musikat/database"
)

func (s *Server) refresh(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	s.tasks.Refresh()

	redirect := "/"
	if next := r.URL.Query().Get("next"); next != "" {
		redirect = next
	}

	http.Redirect(rw, r, redirect, http.StatusSeeOther)
	return nil
}
