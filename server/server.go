package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/component"
	"github.com/shimmerglass/musikat/task"
)

type Server struct {
	cfg Config

	db    *database.DB
	tasks *task.Tasks
}

func New(cfg Config, db *database.DB, tasks *task.Tasks) *Server {
	return &Server{
		cfg:   cfg,
		db:    db,
		tasks: tasks,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /setup", s.handle(s.setupPage))
	mux.Handle("POST /setup", s.handle(s.setupHandler))

	mux.Handle("GET /{$}", s.handle(s.authHandler(s.index)))
	mux.Handle("POST /refresh", s.handle(s.authHandler((s.refresh))))

	mux.Handle("GET /login", s.handle(s.loginPage))
	mux.Handle("POST /login", s.handle(s.loginHandler))
	mux.Handle("POST /logout", s.handle(s.authHandler(s.logout)))

	mux.Handle("GET /account", s.handle(s.authHandler(s.accountPage)))
	mux.Handle("POST /account", s.handle(s.authHandler(s.accountUpdate)))

	mux.Handle("GET /admin/users", s.handle(s.authHandler(s.adminHandler(s.users))))
	mux.Handle("POST /admin/users/delete", s.handle(s.authHandler(s.adminHandler(s.userDelete))))
	mux.Handle("GET /admin/users/create", s.handle(s.authHandler(s.adminHandler(s.userCreatePage))))
	mux.Handle("POST /admin/users/create", s.handle(s.authHandler(s.adminHandler(s.userCreateHandler))))

	mux.Handle("GET /artists/{mbid}", s.handle(s.authHandler(s.artist)))
	mux.Handle("POST /artists/{mbid}/watch", s.handle(s.authHandler(s.watchArtist)))
	mux.Handle("POST /artists/{mbid}/stop-watch", s.handle(s.authHandler(s.stopWatchArtist)))
	mux.Handle("GET /artist-watches/{mbid}", s.handle(s.authHandler(s.artistWatch)))
	mux.Handle("POST /artist-watches/{mbid}", s.handle(s.authHandler(s.artistWatchUpdate)))

	return http.NewCrossOriginProtection().Handler(mux)
}

func (s *Server) Run() error {
	slog.Info("listening", "addr", s.cfg.ListenAddr)
	return http.ListenAndServe(s.cfg.ListenAddr, s.Handler())
}

type handler func(ctx context.Context, rw http.ResponseWriter, r *http.Request) error

func (s *Server) handle(do handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = component.CtxWithURL(ctx, r.URL.Path)
		ctx = component.CtxWithRefreshRunning(ctx, s.tasks.Running())

		err := do(ctx, w, r)
		if err != nil {
			w.WriteHeader(statusFromError(err))
			_ = component.Error(err).Render(ctx, w)
		}
	})
}
