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

func (s *Server) Run() error {
	http.Handle("GET /setup", s.handle(s.setupPage))
	http.Handle("POST /setup", s.handle(s.setupHandler))

	http.Handle("GET /", s.handle(s.authHandler((s.index))))
	http.Handle("GET /refresh", s.handle(s.authHandler((s.refresh))))

	http.Handle("GET /login", s.handle(s.loginPage))
	http.Handle("POST /login", s.handle(s.loginHandler))
	http.Handle("GET /logout", s.handle(s.authHandler(s.logout)))

	http.Handle("GET /account", s.handle(s.authHandler(s.accountPage)))
	http.Handle("POST /account", s.handle(s.authHandler(s.accountUpdate)))

	http.Handle("GET /admin/users", s.handle(s.authHandler(s.users)))
	http.Handle("POST /admin/users/delete", s.handle(s.authHandler(s.userDelete)))
	http.Handle("GET /admin/users/create", s.handle(s.authHandler(s.userCreatePage)))
	http.Handle("POST /admin/users/create", s.handle(s.authHandler(s.userCreateHandler)))

	http.Handle("GET /artists/{mbid}", s.handle(s.authHandler(s.artist)))

	slog.Info("listening", "addr", s.cfg.ListenAddr)
	return http.ListenAndServe(s.cfg.ListenAddr, nil)
}

type handler func(ctx context.Context, rw http.ResponseWriter, r *http.Request) error

func (s *Server) handle(do handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = component.CtxWithURL(ctx, r.URL.Path)
		ctx = component.CtxWithRefreshRunning(ctx, s.tasks.Running())

		err := do(ctx, w, r)
		if err != nil {
			_ = component.Error(err).Render(ctx, w)
		}
	})
}
