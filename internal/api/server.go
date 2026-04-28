// Package api wires the HTTP server.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api/handlers"
	mw "github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/store"
)

type Deps struct {
	Logger        *slog.Logger
	Store         *store.Store
	Disks         handlers.DiskLister
	Pools         handlers.PoolManager
	Datasets      handlers.DatasetManager
	Snapshots     handlers.SnapshotManager
	Dispatcher    *jobs.Dispatcher
	Redis         *redis.Client
	DatasetMgr    *dataset.Manager // concrete manager for streaming send/receive
	PoolMgr       *pool.Manager    // concrete manager for synchronous zpool wait
}

type Server struct {
	deps   Deps
	router chi.Router
}

func New(d Deps) *Server {
	r := chi.NewRouter()
	r.Use(mw.RequestID)
	r.Use(mw.BodyLimit(0)) // 0 = DefaultMaxBody (1 MiB)
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
	poolsWriteH := &handlers.PoolsWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher, Pools: d.Pools}
	dsH := &handlers.DatasetsHandler{Logger: d.Logger, Datasets: d.Datasets}
	dsW := &handlers.DatasetsWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
	dsStream := &handlers.DatasetsStreamHandler{Logger: d.Logger, Dataset: d.DatasetMgr}
	poolWait := &handlers.PoolsWaitHandler{Logger: d.Logger, Pools: d.PoolMgr}
	snapH := &handlers.SnapshotsHandler{Logger: d.Logger, Snapshots: d.Snapshots}
	snapW := &handlers.SnapshotsWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/disks", disksH.List)
		r.Get("/pools", poolsH.List)
		r.Get("/pools/{name}", poolsH.Get)
		r.Post("/pools", poolsWriteH.Create)
		r.Delete("/pools/{name}", poolsWriteH.Destroy)
		r.Post("/pools/{name}/scrub", poolsWriteH.Scrub)
		r.Get("/pools/import", poolsWriteH.Importable)
		r.Post("/pools/import", poolsWriteH.Import)
		r.Post("/pools/{name}/replace", poolsWriteH.Replace)
		r.Post("/pools/{name}/offline", poolsWriteH.Offline)
		r.Post("/pools/{name}/online", poolsWriteH.Online)
		r.Post("/pools/{name}/clear", poolsWriteH.Clear)
		r.Post("/pools/{name}/attach", poolsWriteH.Attach)
		r.Post("/pools/{name}/detach", poolsWriteH.Detach)
		r.Post("/pools/{name}/add", poolsWriteH.Add)
		r.Post("/pools/{name}/export", poolsWriteH.Export)
		r.Post("/pools/{name}/trim", poolsWriteH.Trim)
		r.Patch("/pools/{name}/properties", poolsWriteH.SetProps)
		r.Post("/pools/{name}/wait", poolWait.Wait)
		r.Get("/datasets", dsH.List)
		r.Get("/datasets/{fullname}", dsH.Get)
		r.Post("/datasets", dsW.Create)
		r.Patch("/datasets/{fullname}", dsW.SetProps)
		r.Delete("/datasets/{fullname}", dsW.Destroy)
		r.Post("/datasets/{fullname}/rename", dsW.Rename)
		r.Post("/datasets/{fullname}/clone", dsW.Clone)
		r.Post("/datasets/{fullname}/promote", dsW.Promote)
		r.Post("/datasets/{fullname}/load-key", dsW.LoadKey)
		r.Post("/datasets/{fullname}/unload-key", dsW.UnloadKey)
		r.Post("/datasets/{fullname}/change-key", dsW.ChangeKey)
		r.Post("/datasets/{fullname}/send", dsStream.Send)
		r.Post("/datasets/{fullname}/receive", dsStream.Receive)
		r.Get("/snapshots", snapH.List)
		r.Post("/snapshots", snapW.Create)
		r.Delete("/snapshots/{fullname}", snapW.Destroy)
		r.Post("/datasets/{fullname}/rollback", snapW.Rollback)
		// Jobs routes need the store; construct only when available so
		// tests that build a server without one (e.g. /healthz) still work.
		if d.Store != nil {
			jobsH := &handlers.JobsHandler{Logger: d.Logger, Q: d.Store.Queries}
			r.Get("/jobs", jobsH.List)
			r.Get("/jobs/{id}", jobsH.Get)
			r.Delete("/jobs/{id}", jobsH.Cancel)

			if d.Redis != nil {
				sseH := &handlers.SSEJobsHandler{Logger: d.Logger, Redis: d.Redis, Q: d.Store.Queries}
				r.Get("/jobs/{id}/stream", sseH.Stream)
			}

			metaH := &handlers.MetadataHandler{Logger: d.Logger, Q: d.Store.Queries}
			r.Patch("/pools/{name}/metadata", metaH.PoolPatch)
			r.Patch("/datasets/{fullname}/metadata", metaH.DatasetPatch)
			r.Patch("/snapshots/{fullname}/metadata", metaH.SnapshotPatch)
		}
	})

	return &Server{deps: d, router: r}
}

func (s *Server) Handler() http.Handler { return s.router }
