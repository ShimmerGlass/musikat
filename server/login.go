package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/auth"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) loginPage(ctx context.Context, rw http.ResponseWriter, r *http.Request) error {
	return component.Login().Render(ctx, rw)
}

func (s *Server) loginHandler(ctx context.Context, rw http.ResponseWriter, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	userName := r.Form.Get("user")
	password := r.Form.Get("password")

	user, err := s.db.User(ctx, userName)
	if errors.Is(err, database.ErrUserNotFound) {
		return unauthorized(fmt.Errorf("invalid user or password"))
	}
	if err != nil {
		return err
	}

	ok, err := auth.ComparePasswordAndHash(password, user.Password)
	if err != nil {
		return err
	}
	if !ok {
		return unauthorized(fmt.Errorf("invalid user or password"))
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		UserID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(48 * time.Hour)),
		},
	})
	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(s.cfg.JWTKey))
	if err != nil {
		return err
	}

	var expire time.Time
	if r.Form.Get("remember") == "on" {
		expire = now.Add(24 * 30 * time.Hour)
	}

	http.SetCookie(rw, &http.Cookie{
		Name:     authCookie,
		Value:    tokenString,
		Path:     "/",
		Expires:  expire,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(rw, r, localRedirect(r.URL.Query().Get("next")), http.StatusSeeOther)
	return nil
}

func (s *Server) logout(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	http.SetCookie(rw, &http.Cookie{
		Name:     authCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(rw, r, "/", http.StatusSeeOther)
	return nil
}
