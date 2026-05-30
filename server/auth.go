package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/golang-jwt/jwt/v5"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/component"
)

const authCookie = "token"

type userHandler func(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error

type jwtClaims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

func (s *Server) authHandler(next userHandler) handler {
	return func(ctx context.Context, rw http.ResponseWriter, r *http.Request) error {
		user, ok, err := s.auth(ctx, r)
		if err != nil {
			return err
		}
		if !ok {
			return s.loginRedirect(ctx, rw, r)
		}

		ctx = component.CtxWithUser(ctx, user)

		return next(ctx, rw, r, user)
	}
}

func (s *Server) auth(ctx context.Context, r *http.Request) (database.User, bool, error) {
	cookie, err := r.Cookie(authCookie)
	if errors.Is(err, http.ErrNoCookie) {
		return database.User{}, false, nil
	}

	if cookie.Value == "" {
		return database.User{}, false, nil
	}

	claims := &jwtClaims{}
	_, err = jwt.ParseWithClaims(cookie.Value, claims, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		slog.Warn("invalid token", "err", err)
		return database.User{}, false, nil
	}

	user, err := s.db.UserByID(ctx, claims.UserID)
	if errors.Is(err, database.ErrUserNotFound) {
		return database.User{}, false, nil
	}
	if err != nil {
		return database.User{}, false, err
	}

	return user, true, nil
}

func (s *Server) loginRedirect(ctx context.Context, rw http.ResponseWriter, r *http.Request) error {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		http.Redirect(rw, r, "/setup", http.StatusSeeOther)
	} else {
		http.Redirect(rw, r, fmt.Sprintf("/login?next=%s", url.QueryEscape(r.URL.String())), http.StatusSeeOther)
	}
	return nil
}
