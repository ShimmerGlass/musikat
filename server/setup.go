package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/auth"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) setupPage(ctx context.Context, rw http.ResponseWriter, r *http.Request) error {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		http.Redirect(rw, r, "/", http.StatusSeeOther)
		return nil
	}

	return component.Setup().Render(ctx, rw)
}

func (s *Server) setupHandler(ctx context.Context, rw http.ResponseWriter, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	if r.Form.Get("password") != r.Form.Get("confirm_password") {
		return fmt.Errorf("passwords do not match")
	}

	pass, err := auth.HashPass(r.Form.Get("password"))
	if err != nil {
		return err
	}

	newUser := database.User{
		ID:       r.Form.Get("login"),
		Name:     r.Form.Get("name"),
		Password: pass,
	}
	err = s.db.AddUser(ctx, newUser)
	if err != nil {
		return err
	}

	http.Redirect(rw, r, "/", http.StatusSeeOther)
	return nil
}
