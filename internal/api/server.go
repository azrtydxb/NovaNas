// Package api wires the HTTP server.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/handlers"
	mw "github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/store"
)

type Deps struct {
	Logger *slog.Logger
	Store  *store.Store
	Disks  handlers.DiskLister
}

type Server struct {
	deps   Deps
	router chi.Router
}

func New(d Deps) *Server {
	r := chi.NewRouter()
	r.Use(mw.RequestID)
	r.Use(mw.Recoverer(d.Logger))
	r.Use(mw.Logging(d.Logger))
	r.Get("/healthz", handlers.Healthz)

	disksH := &handlers.DisksHandler{Logger: d.Logger, Lister: d.Disks}
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/disks", disksH.List)
	})

	return &Server{deps: d, router: r}
}

func (s *Server) Handler() http.Handler { return s.router }
