package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

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
		return badRequestf("cannot delete self")
	}

	target, err := s.db.User(ctx, userID)
	if errors.Is(err, database.ErrUserNotFound) {
		return notFound(err)
	}
	if err != nil {
		return err
	}
	if target.Admin {
		users, err := s.db.Users(ctx)
		if err != nil {
			return err
		}
		admins := 0
		for _, u := range users {
			if u.Admin {
				admins++
			}
		}
		if admins <= 1 {
			return badRequestf("cannot delete the last admin")
		}
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

	login := strings.TrimSpace(r.Form.Get("login"))
	if login == "" {
		return badRequestf("login is required")
	}
	if r.Form.Get("password") == "" {
		return badRequestf("password is required")
	}

	pass, err := auth.HashPass(r.Form.Get("password"))
	if err != nil {
		return err
	}

	newUser := database.User{
		ID:       login,
		Name:     r.Form.Get("name"),
		Password: pass,
		Admin:    r.Form.Get("admin") == "on",
	}
	err = s.db.CreateUser(ctx, newUser)
	if errors.Is(err, database.ErrUserExists) {
		return badRequestf("user already exists")
	}
	if err != nil {
		return err
	}

	http.Redirect(rw, r, "/admin/users", http.StatusSeeOther)
	return nil
}
