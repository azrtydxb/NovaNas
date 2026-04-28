// Package api wires the HTTP server.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api/handlers"
	mw "github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/store"
)

type Deps struct {
	Logger     *slog.Logger
	Store      *store.Store
	Disks      handlers.DiskLister
	Pools      handlers.PoolManager
	Datasets   handlers.DatasetManager
	Snapshots  handlers.SnapshotManager
	Dispatcher *jobs.Dispatcher
	Redis      *redis.Client
}

type Server struct {
	deps   Deps
	router chi.Router
}

func New(d Deps) *Server {
	r := chi.NewRouter()
	r.Use(mw.RequestID)
	// Audit is registered before Recoverer so that a panicking handler
	// still writes its audit row (Recoverer catches the panic and writes
	// the 500, then control returns up to Audit's post-`next` block,
	// which observes the captured status and inserts the row).
	if d.Store != nil {
		r.Use(mw.Audit(d.Store.Queries, d.Logger))
	}
	r.Use(mw.Recoverer(d.Logger))
	r.Use(mw.Logging(d.Logger))
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		mw.WriteError(w, http.StatusNotFound, "not_found", "no such route")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		mw.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed for this route")
	})
	r.Get("/healthz", handlers.Healthz)

	disksH := &handlers.DisksHandler{Logger: d.Logger, Lister: d.Disks}
	poolsH := &handlers.PoolsHandler{Logger: d.Logger, Pools: d.Pools}
	dsH := &handlers.DatasetsHandler{Logger: d.Logger, Datasets: d.Datasets}
	snapH := &handlers.SnapshotsHandler{Logger: d.Logger, Snapshots: d.Snapshots}
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/disks", disksH.List)
		r.Get("/pools", poolsH.List)
		r.Get("/pools/{name}", poolsH.Get)
		r.Get("/datasets", dsH.List)
		r.Get("/datasets/{fullname}", dsH.Get)
		r.Get("/snapshots", snapH.List)
	})

	return &Server{deps: d, router: r}
}

func (s *Server) Handler() http.Handler { return s.router }
