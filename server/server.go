package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/component"
)

type Server struct {
	cfg Config

	db *database.DB
}

func New(cfg Config, db *database.DB) *Server {
	return &Server{
		cfg: cfg,
		db:  db,
	}
}

func (s *Server) Run() error {
	http.Handle("GET /setup", handle(s.setupPage))
	http.Handle("POST /setup", handle(s.setupHandler))

	http.Handle("GET /", handle(s.authHandler((s.index))))

	http.Handle("GET /login", handle(s.loginPage))
	http.Handle("POST /login", handle(s.loginHandler))
	http.Handle("GET /logout", handle(s.authHandler(s.logout)))

	http.Handle("GET /account", handle(s.authHandler(s.accountPage)))
	http.Handle("POST /account", handle(s.authHandler(s.accountUpdate)))

	http.Handle("GET /admin/users", handle(s.authHandler(s.users)))
	http.Handle("POST /admin/users/delete", handle(s.authHandler(s.userDelete)))
	http.Handle("GET /admin/users/create", handle(s.authHandler(s.userCreatePage)))
	http.Handle("POST /admin/users/create", handle(s.authHandler(s.userCreateHandler)))

	http.Handle("GET /artists/{mbid}", handle(s.authHandler(s.artist)))

	slog.Info("listening", "addr", s.cfg.ListenAddr)
	return http.ListenAndServe(s.cfg.ListenAddr, nil)
}

type handler func(ctx context.Context, rw http.ResponseWriter, r *http.Request) error

func handle(do handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = component.CtxWithURL(ctx, r.URL.Path)

		err := do(ctx, w, r)
		if err != nil {
			component.Error(err).Render(ctx, w)
		}
	})
}
