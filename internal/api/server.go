// Package api wires the HTTP server.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api/handlers"
	"github.com/novanas/nova-nas/internal/api/metrics"
	mw "github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/auth"
	"github.com/novanas/nova-nas/internal/host/iscsi"
	"github.com/novanas/nova-nas/internal/host/krb5"
	"github.com/novanas/nova-nas/internal/host/network"
	"github.com/novanas/nova-nas/internal/host/nfs"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
	"github.com/novanas/nova-nas/internal/host/protocolshare"
	"github.com/novanas/nova-nas/internal/host/rdma"
	"github.com/novanas/nova-nas/internal/host/samba"
	"github.com/novanas/nova-nas/internal/host/scheduler"
	"github.com/novanas/nova-nas/internal/host/secrets"
	"github.com/novanas/nova-nas/internal/host/smart"
	"github.com/novanas/nova-nas/internal/host/system"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/store"
)

type Deps struct {
	Logger        *slog.Logger
	Store         *store.Store
	Disks         handlers.DiskLister
	Pools         handlers.PoolManager
	Datasets      handlers.DatasetManager
	Snapshots     handlers.SnapshotManager
	Dispatcher    handlers.Dispatcher
	Redis         *redis.Client
	DatasetMgr    *dataset.Manager   // concrete manager for streaming send/receive and queries (diff, bookmarks)
	PoolMgr       *pool.Manager      // concrete manager for synchronous zpool wait/sync
	SnapshotMgr   *snapshot.Manager  // concrete manager for synchronous holds queries
	IscsiMgr      *iscsi.Manager
	NvmeofMgr     *nvmeof.Manager
	NfsMgr        *nfs.Manager
	Krb5Mgr       *krb5.Manager
	Krb5KDC       *krb5.KDCManager
	RdmaLister    *rdma.Lister
	SambaMgr      *samba.Manager
	SmartMgr      *smart.Manager
	SchedulerMgr  *scheduler.Manager
	NetworkMgr    *network.Manager
	SystemMgr     *system.Manager
	ProtocolShareMgr *protocolshare.Manager

	// Auth wiring. If Verifier is nil OR AuthDisabled is true, the
	// /api/v1 group does not enforce authentication or per-route
	// permissions (e.g. for tests, dev). In production both are required.
	Verifier     *auth.Verifier
	RoleMap      auth.RoleMap
	AuthDisabled bool

	// Secrets is the runtime secret-storage manager. Optional; passed
	// here so future handlers/jobs can read/write secrets without
	// reaching into globals.
	Secrets secrets.Manager

	// Metrics, when non-nil, installs a request-instrumentation middleware
	// and mounts /metrics on the main router. The same Metrics handle is
	// also wired through to the dispatcher and worker so the three
	// collector groups (HTTP, jobs, ZFS) share a single registry.
	//
	// MetricsHandler overrides the served handler — set it to nil when
	// /metrics is exposed on a separate listener (see METRICS_ADDR) so the
	// main listener does NOT also expose the endpoint.
	Metrics        *metrics.Metrics
	MetricsHandler http.Handler
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
	// Prometheus instrumentation. Wired before the per-domain RBAC groups
	// so 401/403 short-circuits are still observed in the request counter.
	// The middleware itself excludes /metrics, so a scrape doesn't bump
	// its own counter.
	if d.Metrics != nil {
		r.Use(d.Metrics.HTTP.Middleware)
	}
	// /metrics is mounted at the root (Prometheus convention) and is
	// public. Operators that don't want it on the main API listener
	// should set METRICS_ADDR to bind it to a separate listener — in
	// which case Deps.MetricsHandler is set to nil.
	if d.MetricsHandler != nil {
		r.Method(http.MethodGet, "/metrics", d.MetricsHandler)
	}
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		mw.WriteError(w, http.StatusNotFound, "not_found", "no such route")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		mw.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed for this route")
	})
	// /healthz is public — no auth applied.
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
	dsQuery := &handlers.DatasetsQueryHandler{Logger: d.Logger, Dataset: d.DatasetMgr}
	snapHolds := &handlers.SnapshotsHoldsHandler{Logger: d.Logger, Snapshot: d.SnapshotMgr}
	poolSync := &handlers.PoolsSyncHandler{Logger: d.Logger, Pool: d.PoolMgr}
	var iscsiH *handlers.IscsiHandler
	var iscsiW *handlers.IscsiWriteHandler
	if d.IscsiMgr != nil {
		iscsiH = &handlers.IscsiHandler{Logger: d.Logger, Mgr: d.IscsiMgr}
	}
	iscsiW = &handlers.IscsiWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
	var nvmeofH *handlers.NvmeofHandler
	var nvmeofW *handlers.NvmeofWriteHandler
	if d.NvmeofMgr != nil {
		nvmeofH = &handlers.NvmeofHandler{Logger: d.Logger, Mgr: d.NvmeofMgr}
	}
	nvmeofW = &handlers.NvmeofWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
	var nfsH *handlers.NfsHandler
	if d.NfsMgr != nil {
		nfsH = &handlers.NfsHandler{Logger: d.Logger, Mgr: d.NfsMgr}
	}
	nfsW := &handlers.NfsWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
	var krb5H *handlers.Krb5Handler
	if d.Krb5Mgr != nil {
		krb5H = &handlers.Krb5Handler{Logger: d.Logger, Mgr: d.Krb5Mgr}
	}
	krb5W := &handlers.Krb5WriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
	var krb5KDCH *handlers.Krb5KDCHandler
	if d.Krb5KDC != nil {
		krb5KDCH = &handlers.Krb5KDCHandler{Logger: d.Logger, KDC: d.Krb5KDC}
	}
	rdmaH := &handlers.RDMAHandler{Logger: d.Logger, Lister: d.RdmaLister}
	var sambaH *handlers.SambaHandler
	if d.SambaMgr != nil {
		sambaH = &handlers.SambaHandler{Logger: d.Logger, Mgr: d.SambaMgr}
	}
	sambaW := &handlers.SambaWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
	var smartH *handlers.SmartHandler
	if d.SmartMgr != nil {
		smartH = &handlers.SmartHandler{Logger: d.Logger, Mgr: d.SmartMgr, Dispatcher: d.Dispatcher}
	}
	var networkH *handlers.NetworkHandler
	var networkW *handlers.NetworkWriteHandler
	if d.NetworkMgr != nil {
		networkH = &handlers.NetworkHandler{Logger: d.Logger, Mgr: d.NetworkMgr}
		networkW = &handlers.NetworkWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher, Mgr: d.NetworkMgr}
	}
	var systemH *handlers.SystemHandler
	if d.SystemMgr != nil {
		systemH = &handlers.SystemHandler{Logger: d.Logger, Mgr: d.SystemMgr, Dispatcher: d.Dispatcher}
	}
	var psH *handlers.ProtocolShareHandler
	if d.ProtocolShareMgr != nil {
		psH = &handlers.ProtocolShareHandler{Logger: d.Logger, Mgr: d.ProtocolShareMgr}
	}
	psW := &handlers.ProtocolShareWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
	var dsACLH *handlers.DatasetACLHandler
	if d.DatasetMgr != nil {
		dsACLH = &handlers.DatasetACLHandler{Logger: d.Logger, Dataset: d.DatasetMgr, Dispatcher: d.Dispatcher}
	}
	var sambaGlobalsH *handlers.SambaGlobalsHandler
	if d.SambaMgr != nil {
		sambaGlobalsH = &handlers.SambaGlobalsHandler{Logger: d.Logger, Mgr: d.SambaMgr, Dispatcher: d.Dispatcher}
	}

	// authEnabled controls whether auth middleware and per-route
	// permission gates are installed. When disabled (test mode), routes
	// behave as before.
	authEnabled := d.Verifier != nil && !d.AuthDisabled
	roleMap := d.RoleMap
	if roleMap == nil {
		roleMap = auth.DefaultRoleMap
	}

	// require returns a middleware chain enforcing perm. When auth is
	// disabled it is a no-op so existing handler tests work unchanged.
	require := func(p auth.Permission) func(http.Handler) http.Handler {
		if !authEnabled {
			return func(next http.Handler) http.Handler { return next }
		}
		return auth.RequirePermission(roleMap, p)
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Authenticate every /api/v1/* request.
		if authEnabled {
			r.Use(d.Verifier.Middleware(nil, d.Logger))
		}

		// /auth/me — any authenticated identity.
		r.Get("/auth/me", authMeHandler)

		// ---- Storage read ----
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermStorageRead))
			r.Get("/disks", disksH.List)
			r.Get("/pools", poolsH.List)
			r.Get("/pools/{name}", poolsH.Get)
			r.Get("/pools/import", poolsWriteH.Importable)
			r.Get("/datasets", dsH.List)
			r.Get("/datasets/{fullname}", dsH.Get)
			r.Get("/datasets/{fullname}/diff", dsQuery.Diff)
			r.Get("/datasets/{fullname}/bookmarks", dsQuery.ListBookmarks)
			r.Get("/snapshots", snapH.List)
			r.Get("/snapshots/{fullname}/holds", snapHolds.Holds)
			if iscsiH != nil {
				r.Get("/iscsi/targets", iscsiH.ListTargets)
				r.Get("/iscsi/targets/{iqn}", iscsiH.GetTarget)
			}
			if nvmeofH != nil {
				r.Get("/nvmeof/subsystems", nvmeofH.ListSubsystems)
				r.Get("/nvmeof/subsystems/{nqn}", nvmeofH.GetSubsystem)
				r.Get("/nvmeof/ports", nvmeofH.ListPorts)
				r.Get("/nvmeof/hosts/{nqn}/dhchap", nvmeofH.GetHostDHChap)
			}
			if nfsH != nil {
				r.Get("/nfs/exports", nfsH.ListExports)
				r.Get("/nfs/exports/active", nfsH.ListActive)
				r.Get("/nfs/exports/{name}", nfsH.GetExport)
			}
			if krb5H != nil {
				r.Get("/krb5/config", krb5H.GetConfig)
				r.Get("/krb5/idmapd", krb5H.GetIdmapd)
				r.Get("/krb5/keytab", krb5H.ListKeytab)
			}
			r.Get("/network/rdma", rdmaH.List)
			if sambaH != nil {
				r.Get("/samba/shares", sambaH.ListShares)
				r.Get("/samba/shares/{name}", sambaH.GetShare)
				r.Get("/samba/users", sambaH.ListUsers)
			}
			if sambaGlobalsH != nil {
				r.Get("/samba/globals", sambaGlobalsH.Get)
			}
			if psH != nil {
				r.Get("/protocol-shares", psH.List)
				r.Get("/protocol-shares/{name}", psH.Get)
			}
			if dsACLH != nil {
				r.Get("/datasets/{fullname}/acl", dsACLH.Get)
			}
			if smartH != nil {
				r.Get("/disks/{name}/smart", smartH.Get)
			}
		})

		// ---- Storage write ----
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermStorageWrite))
			r.Post("/pools", poolsWriteH.Create)
			r.Delete("/pools/{name}", poolsWriteH.Destroy)
			r.Post("/pools/{name}/scrub", poolsWriteH.Scrub)
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
			r.Post("/pools/{name}/checkpoint", poolsWriteH.Checkpoint)
			r.Post("/pools/{name}/discard-checkpoint", poolsWriteH.DiscardCheckpoint)
			r.Post("/pools/{name}/upgrade", poolsWriteH.Upgrade)
			r.Post("/pools/{name}/reguid", poolsWriteH.Reguid)
			r.Post("/pools/sync", poolSync.Sync)

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
			r.Post("/datasets/{fullname}/bookmark", dsW.Bookmark)
			r.Post("/datasets/{fullname}/destroy-bookmark", dsW.DestroyBookmark)

			r.Post("/snapshots", snapW.Create)
			r.Delete("/snapshots/{fullname}", snapW.Destroy)
			r.Post("/datasets/{fullname}/rollback", snapW.Rollback)
			r.Post("/snapshots/{fullname}/hold", snapW.Hold)
			r.Post("/snapshots/{fullname}/release", snapW.Release)

			// iSCSI write
			r.Post("/iscsi/targets", iscsiW.CreateTarget)
			r.Delete("/iscsi/targets/{iqn}", iscsiW.DestroyTarget)
			r.Post("/iscsi/targets/{iqn}/portals", iscsiW.CreatePortal)
			r.Delete("/iscsi/targets/{iqn}/portals/{ip}/{port}", iscsiW.DeletePortal)
			r.Post("/iscsi/targets/{iqn}/luns", iscsiW.CreateLUN)
			r.Delete("/iscsi/targets/{iqn}/luns/{id}", iscsiW.DeleteLUN)
			r.Post("/iscsi/targets/{iqn}/acls", iscsiW.CreateACL)
			r.Delete("/iscsi/targets/{iqn}/acls/{initiatorIqn}", iscsiW.DeleteACL)
			r.Post("/iscsi/saveconfig", iscsiW.SaveConfig)

			// NVMe-oF write
			r.Post("/nvmeof/subsystems", nvmeofW.CreateSubsystem)
			r.Delete("/nvmeof/subsystems/{nqn}", nvmeofW.DestroySubsystem)
			r.Post("/nvmeof/subsystems/{nqn}/namespaces", nvmeofW.AddNamespace)
			r.Delete("/nvmeof/subsystems/{nqn}/namespaces/{nsid}", nvmeofW.RemoveNamespace)
			r.Post("/nvmeof/subsystems/{nqn}/hosts", nvmeofW.AllowHost)
			r.Delete("/nvmeof/subsystems/{nqn}/hosts/{hostNqn}", nvmeofW.DisallowHost)
			r.Post("/nvmeof/ports", nvmeofW.CreatePort)
			r.Delete("/nvmeof/ports/{id}", nvmeofW.DeletePort)
			r.Post("/nvmeof/ports/{id}/subsystems", nvmeofW.LinkSubsystem)
			r.Delete("/nvmeof/ports/{id}/subsystems/{nqn}", nvmeofW.UnlinkSubsystem)
			r.Post("/nvmeof/hosts/{nqn}/dhchap", nvmeofW.SetHostDHChap)
			r.Delete("/nvmeof/hosts/{nqn}/dhchap", nvmeofW.ClearHostDHChap)
			r.Post("/nvmeof/saveconfig", nvmeofW.SaveConfig)

			// NFS write
			r.Post("/nfs/exports", nfsW.CreateExport)
			r.Patch("/nfs/exports/{name}", nfsW.UpdateExport)
			r.Delete("/nfs/exports/{name}", nfsW.DeleteExport)
			r.Post("/nfs/reload", nfsW.Reload)

			// Kerberos write
			r.Put("/krb5/config", krb5W.SetConfig)
			r.Put("/krb5/idmapd", krb5W.SetIdmapd)
			r.Put("/krb5/keytab", krb5W.UploadKeytab)
			r.Delete("/krb5/keytab", krb5W.DeleteKeytab)

			// Samba write
			r.Post("/samba/shares", sambaW.CreateShare)
			r.Patch("/samba/shares/{name}", sambaW.UpdateShare)
			r.Delete("/samba/shares/{name}", sambaW.DeleteShare)
			r.Post("/samba/reload", sambaW.Reload)
			r.Post("/samba/users", sambaW.AddUser)
			r.Delete("/samba/users/{username}", sambaW.DeleteUser)
			r.Put("/samba/users/{username}/password", sambaW.SetUserPassword)
			if sambaGlobalsH != nil {
				r.Put("/samba/globals", sambaGlobalsH.Set)
			}

			// ProtocolShare write
			r.Post("/protocol-shares", psW.Create)
			r.Patch("/protocol-shares/{name}", psW.Update)
			r.Delete("/protocol-shares/{name}", psW.Delete)

			// Dataset NFSv4 ACL write
			if dsACLH != nil {
				r.Put("/datasets/{fullname}/acl", dsACLH.Set)
				r.Post("/datasets/{fullname}/acl/append", dsACLH.Append)
				r.Delete("/datasets/{fullname}/acl/{index}", dsACLH.Remove)
			}

			// SMART write
			if smartH != nil {
				r.Post("/disks/{name}/smart/test", smartH.RunSelfTest)
				r.Post("/disks/{name}/smart/enable", smartH.Enable)
			}
		})

		// ---- Krb5 KDC read (principal listing + status) ----
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermKrb5Read))
			if krb5KDCH != nil {
				r.Get("/krb5/kdc/status", krb5KDCH.GetStatus)
				r.Get("/krb5/principals", krb5KDCH.ListPrincipals)
				r.Get("/krb5/principals/{name}", krb5KDCH.GetPrincipal)
			}
		})

		// ---- Krb5 KDC write (principal CRUD + keytab issuance) ----
		// Keytab generation is gated on PermKrb5Write because the
		// returned bytes ARE the credential.
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermKrb5Write))
			if krb5KDCH != nil {
				r.Post("/krb5/principals", krb5KDCH.CreatePrincipal)
				r.Delete("/krb5/principals/{name}", krb5KDCH.DeletePrincipal)
				r.Post("/krb5/principals/{name}/keytab", krb5KDCH.GetPrincipalKeytab)
			}
		})

		// ---- Network read ----
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermNetworkRead))
			if networkH != nil {
				r.Get("/network/interfaces", networkH.ListInterfaces)
				r.Get("/network/configs", networkH.ListConfigs)
				r.Get("/network/configs/{name}", networkH.GetConfig)
			}
		})

		// ---- Network write ----
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermNetworkWrite))
			if networkW != nil {
				r.Post("/network/configs", networkW.ApplyInterface)
				r.Delete("/network/configs/{name}", networkW.DeleteInterface)
				r.Post("/network/vlans", networkW.ApplyVLAN)
				r.Post("/network/bonds", networkW.ApplyBond)
				r.Post("/network/reload", networkW.Reload)
			}
		})

		// ---- System read ----
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermSystemRead))
			if systemH != nil {
				r.Get("/system/info", systemH.GetInfo)
				r.Get("/system/time", systemH.GetTime)
			}
		})

		// ---- System write ----
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermSystemWrite))
			if systemH != nil {
				r.Put("/system/hostname", systemH.SetHostname)
				r.Put("/system/timezone", systemH.SetTimezone)
				r.Put("/system/ntp", systemH.SetNTP)
			}
		})

		// ---- System admin (reboot/shutdown) ----
		r.Group(func(r chi.Router) {
			r.Use(require(auth.PermSystemAdmin))
			if systemH != nil {
				r.Post("/system/reboot", systemH.Reboot)
				r.Post("/system/shutdown", systemH.Shutdown)
				r.Post("/system/cancel-shutdown", systemH.CancelShutdown)
			}
		})

		// Jobs and scheduler — only available when the store is wired.
		if d.Store != nil {
			jobsH := &handlers.JobsHandler{Logger: d.Logger, Q: d.Store.Queries}
			// Jobs span domains; default to storage:read.
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermStorageRead))
				r.Get("/jobs", jobsH.List)
				r.Get("/jobs/{id}", jobsH.Get)
				r.Delete("/jobs/{id}", jobsH.Cancel)
				if d.Redis != nil {
					sseH := &handlers.SSEJobsHandler{Logger: d.Logger, Redis: d.Redis, Q: d.Store.Queries}
					r.Get("/jobs/{id}/stream", sseH.Stream)
				}
			})

			schedH := &handlers.SchedulerHandler{Logger: d.Logger, Q: d.Store.Queries}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermSchedulerRead))
				r.Get("/scheduler/snapshot-schedules", schedH.ListSnapshotSchedules)
				r.Get("/scheduler/snapshot-schedules/{id}", schedH.GetSnapshotSchedule)
				r.Get("/scheduler/replication-targets", schedH.ListReplicationTargets)
				r.Get("/scheduler/replication-targets/{id}", schedH.GetReplicationTarget)
				r.Get("/scheduler/replication-schedules", schedH.ListReplicationSchedules)
				r.Get("/scheduler/replication-schedules/{id}", schedH.GetReplicationSchedule)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermSchedulerWrite))
				r.Post("/scheduler/snapshot-schedules", schedH.CreateSnapshotSchedule)
				r.Patch("/scheduler/snapshot-schedules/{id}", schedH.UpdateSnapshotSchedule)
				r.Delete("/scheduler/snapshot-schedules/{id}", schedH.DeleteSnapshotSchedule)
				r.Post("/scheduler/replication-targets", schedH.CreateReplicationTarget)
				r.Delete("/scheduler/replication-targets/{id}", schedH.DeleteReplicationTarget)
				r.Post("/scheduler/replication-schedules", schedH.CreateReplicationSchedule)
				r.Patch("/scheduler/replication-schedules/{id}", schedH.UpdateReplicationSchedule)
				r.Delete("/scheduler/replication-schedules/{id}", schedH.DeleteReplicationSchedule)
			})

			metaH := &handlers.MetadataHandler{Logger: d.Logger, Q: d.Store.Queries}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermStorageWrite))
				r.Patch("/pools/{name}/metadata", metaH.PoolPatch)
				r.Patch("/datasets/{fullname}/metadata", metaH.DatasetPatch)
				r.Patch("/snapshots/{fullname}/metadata", metaH.SnapshotPatch)
			})
		}
	})

	return &Server{deps: d, router: r}
}

func (s *Server) Handler() http.Handler { return s.router }

// authMeHandler returns the authenticated identity attached to the
// request context by the auth middleware. When auth is disabled it
// returns 200 with an empty body, since no identity is enforced.
func authMeHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := auth.IdentityFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if !ok || id == nil {
		// Auth disabled (or unreachable): return empty object so callers
		// can still distinguish from 401.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(id)
}
