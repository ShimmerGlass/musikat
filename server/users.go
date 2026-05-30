package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/auth"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) users(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	users, err := s.db.Users(ctx)
	if err != nil {
		return err
	}

	return component.Users(users).Render(ctx, rw)
}

func (s *Server) userDelete(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	userID := r.Form.Get("user_id")
	if userID == user.ID {
		return fmt.Errorf("cannot delete self")
	}

	err = s.db.UserDelete(ctx, userID)
	if err != nil {
		return err
	}

	http.Redirect(rw, r, "/admin/users", http.StatusSeeOther)
	return nil
}

func (s *Server) userCreatePage(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	return component.CreateUser().Render(ctx, rw)
}

func (s *Server) userCreateHandler(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	err := r.ParseForm()
	if err != nil {
		return err
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

	http.Redirect(rw, r, "/admin/users", http.StatusSeeOther)
	return nil
}
