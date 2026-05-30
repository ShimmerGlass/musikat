package server

import (
	"context"
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

	user, err := s.db.UserByID(ctx, userName)
	if err != nil {
		return err
	}

	ok, err := auth.ComparePasswordAndHash(password, user.Password)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("invalid password")
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

	http.SetCookie(rw, &http.Cookie{
		Name:     authCookie,
		Value:    tokenString,
		Path:     "/",
		Expires:  now.Add(48 * time.Hour),
		HttpOnly: true,
	})

	redirect := "/"
	if next := r.URL.Query().Get("next"); next != "" {
		redirect = next
	}

	http.Redirect(rw, r, redirect, http.StatusSeeOther)
	return nil
}

func (s *Server) logout(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	http.SetCookie(rw, &http.Cookie{
		Name:     authCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Time{},
		HttpOnly: true,
	})

	http.Redirect(rw, r, "/", http.StatusSeeOther)
	return nil
}
