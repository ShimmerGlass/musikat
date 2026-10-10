package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/auth"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) setupPage(ctx context.Context, rw http.ResponseWriter, r *http.Request) error {
	users, err := s.db.Users(ctx)
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
	users, err := s.db.Users(ctx)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return forbidden(fmt.Errorf("setup already completed"))
	}

	err = r.ParseForm()
	if err != nil {
		return badRequest(err)
	}

	login := strings.TrimSpace(r.Form.Get("login"))
	if login == "" {
		return badRequestf("login is required")
	}
	if r.Form.Get("password") == "" {
		return badRequestf("password is required")
	}
	if r.Form.Get("password") != r.Form.Get("confirm_password") {
		return badRequestf("passwords do not match")
	}

	pass, err := auth.HashPass(r.Form.Get("password"))
	if err != nil {
		return err
	}

	newUser := database.User{
		ID:       login,
		Name:     r.Form.Get("name"),
		Password: pass,
		Admin:    true,
	}
	err = s.db.CreateFirstUser(ctx, newUser)
	if errors.Is(err, database.ErrSetupDone) {
		return forbidden(fmt.Errorf("setup already completed"))
	}
	if err != nil {
		return err
	}

	http.Redirect(rw, r, "/", http.StatusSeeOther)
	return nil
}
