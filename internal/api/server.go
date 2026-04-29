// Package api wires the HTTP server.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

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
	notifysmtp "github.com/novanas/nova-nas/internal/host/notify/smtp"
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
	"github.com/novanas/nova-nas/internal/notifycenter"
	"github.com/novanas/nova-nas/internal/plugins"
	"github.com/novanas/nova-nas/internal/replication"
	"github.com/novanas/nova-nas/internal/store"
	"github.com/novanas/nova-nas/internal/vms"
	"github.com/novanas/nova-nas/internal/workloads"
)

type Deps struct {
	Logger           *slog.Logger
	Store            *store.Store
	Disks            handlers.DiskLister
	Pools            handlers.PoolManager
	Datasets         handlers.DatasetManager
	Snapshots        handlers.SnapshotManager
	Dispatcher       handlers.Dispatcher
	Redis            *redis.Client
	DatasetMgr       *dataset.Manager  // concrete manager for streaming send/receive and queries (diff, bookmarks)
	PoolMgr          *pool.Manager     // concrete manager for synchronous zpool wait/sync
	SnapshotMgr      *snapshot.Manager // concrete manager for synchronous holds queries
	IscsiMgr         *iscsi.Manager
	NvmeofMgr        *nvmeof.Manager
	NfsMgr           *nfs.Manager
	Krb5Mgr          *krb5.Manager
	Krb5KDC          *krb5.KDCManager
	RdmaLister       *rdma.Lister
	SambaMgr         *samba.Manager
	SmartMgr         *smart.Manager
	SchedulerMgr     *scheduler.Manager
	NetworkMgr       *network.Manager
	SystemMgr        *system.Manager
	ProtocolShareMgr *protocolshare.Manager
	SMTPMgr          *notifysmtp.Manager
	// NotifyCenter is the unified Notification Center manager (the
	// bell). When nil the /notifications/events* routes are not
	// mounted. Distinct from SMTPMgr (outbound email).
	NotifyCenter *notifycenter.Manager
	// EncryptionMgr is the TPM-sealed ZFS native-encryption manager.
	// When nil, the /encryption* endpoints return 503.
	EncryptionMgr *dataset.EncryptionManager

	// ReplicationMgr is the general-replication subsystem manager. When
	// nil the /replication-jobs endpoints are not mounted.
	ReplicationMgr *replication.Manager

	// VMMgr is the KubeVirt-backed VM manager. When nil the /vms,
	// /vm-templates, /vm-snapshots, /vm-restores routes are mounted but
	// respond 503 — the GUI degrades gracefully on hosts without a
	// working KubeVirt control plane.
	VMMgr *vms.Manager

	// Workloads subsystem (Helm-driven Apps lifecycle on the embedded
	// k3s). When nil the /workloads/* routes are mounted but respond
	// 503 — the Package Center GUI degrades gracefully on hosts where
	// k3s hasn't been bootstrapped yet.
	WorkloadsMgr workloads.Lifecycle

	// Tier 2 plugin engine. PluginsMgr orchestrates install/uninstall/
	// upgrade; PluginsRouter mounts reverse-proxy routes for installed
	// plugins; PluginsUI serves the static UI bundles; PluginsMarket
	// is the marketplace client. All four are typically wired together
	// in cmd/nova-api/main.go from the same env-derived config; tests
	// may wire only the subset they exercise.
	PluginsMgr     *plugins.Manager
	PluginsRouter  *plugins.Router
	PluginsUI      *plugins.UIAssets
	PluginsMarket  *plugins.MarketplaceClient
	// MarketplacesStore + MarketplacesMulti drive the multi-source
	// marketplace registry surfaced at /marketplaces. When Store is
	// nil the routes respond 503.
	MarketplacesStore plugins.MarketplacesStore
	MarketplacesMulti *plugins.MultiMarketplaceClient

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

	// AlertsHandler, when non-nil, mounts /api/v1/alerts*,
	// /api/v1/alert-silences*, /api/v1/alert-receivers as a pass-through
	// to the upstream Alertmanager.
	AlertsHandler *handlers.AlertsHandler
	// LogsHandler, when non-nil, mounts /api/v1/logs/* as a pass-through
	// to the upstream Loki.
	LogsHandler *handlers.LogsHandler
	// SessionsHandler, when non-nil, mounts /api/v1/auth/sessions*,
	// /api/v1/auth/users/{id}/sessions, and /api/v1/auth/login-history*
	// as Keycloak admin pass-throughs.
	SessionsHandler *handlers.SessionsHandler
	// SystemMetaHandler, when non-nil, mounts /api/v1/system/version
	// and /api/v1/system/updates.
	SystemMetaHandler *handlers.SystemMetaHandler

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
	var notifyH *handlers.NotificationsHandler
	if d.SMTPMgr != nil {
		notifyH = &handlers.NotificationsHandler{Logger: d.Logger, Mgr: d.SMTPMgr}
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

		// ---- Dataset native encryption (TPM-sealed key escrow) ----
		// Initialize/load/unload require Write; Recover (returns the
		// raw key in the response body) requires the dedicated
		// admin-only Recover permission and is audit-logged by the
		// handler in addition to the global audit middleware.
		{
			var encAuditor handlers.EncryptionAuditor
			if d.Store != nil {
				encAuditor = d.Store.Queries
			}
			encH := &handlers.EncryptionHandler{
				Logger:  d.Logger,
				Mgr:     d.EncryptionMgr,
				Auditor: encAuditor,
			}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermPoolEncryptionWrite))
				r.Post("/datasets/{fullname}/encryption", encH.Initialize)
				r.Post("/datasets/{fullname}/encryption/load-key", encH.LoadKey)
				r.Post("/datasets/{fullname}/encryption/unload-key", encH.UnloadKey)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermPoolEncryptionRecover))
				r.Post("/datasets/{fullname}/encryption/recover", encH.Recover)
			})
		}

		// ---- Notifications (SMTP relay) ----
		if notifyH != nil {
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermNotificationsRead))
				r.Get("/notifications/smtp", notifyH.GetConfig)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermNotificationsWrite))
				r.Put("/notifications/smtp", notifyH.PutConfig)
				r.Post("/notifications/smtp/test", notifyH.PostTest)
			})
		}

		// ---- Notification Center (the bell) ----
		// Distinct prefix (/notifications/events) from the SMTP relay
		// (/notifications/smtp). The two share a parent path but are
		// otherwise unrelated subsystems.
		if d.NotifyCenter != nil {
			nceH := &handlers.NotificationsEventsHandler{Logger: d.Logger, Mgr: d.NotifyCenter}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermNotificationsEventsRead))
				r.Get("/notifications/events", nceH.List)
				r.Get("/notifications/events/unread-count", nceH.UnreadCount)
				r.Get("/notifications/events/stream", nceH.Stream)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermNotificationsEventsWrite))
				r.Post("/notifications/events/{id}/read", nceH.MarkRead)
				r.Post("/notifications/events/{id}/dismiss", nceH.MarkDismissed)
				r.Post("/notifications/events/{id}/snooze", nceH.Snooze)
				r.Post("/notifications/events/read-all", nceH.MarkAllRead)
			})
		}

		// ---- KubeVirt VM management ----
		// Always mounted; when VMMgr is nil the handlers respond 503 so
		// the GUI gets a stable shape regardless of cluster state.
		{
			vmsH := &handlers.VMsHandler{Logger: d.Logger, Mgr: d.VMMgr}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermVMRead))
				r.Get("/vms", vmsH.List)
				r.Get("/vms/{namespace}/{name}", vmsH.Get)
				r.Get("/vms/{namespace}/{name}/console", vmsH.Console)
				r.Get("/vms/{namespace}/{name}/serial", vmsH.Serial)
				r.Get("/vm-templates", vmsH.ListTemplates)
				r.Get("/vm-snapshots", vmsH.ListSnapshots)
				r.Get("/vm-restores", vmsH.ListRestores)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermVMWrite))
				r.Post("/vms", vmsH.Create)
				r.Patch("/vms/{namespace}/{name}", vmsH.Patch)
				r.Delete("/vms/{namespace}/{name}", vmsH.Delete)
				r.Post("/vms/{namespace}/{name}/start", vmsH.Start)
				r.Post("/vms/{namespace}/{name}/stop", vmsH.Stop)
				r.Post("/vms/{namespace}/{name}/restart", vmsH.Restart)
				r.Post("/vms/{namespace}/{name}/pause", vmsH.Pause)
				r.Post("/vms/{namespace}/{name}/unpause", vmsH.Unpause)
				r.Post("/vms/{namespace}/{name}/migrate", vmsH.Migrate)
				r.Post("/vm-snapshots", vmsH.CreateSnapshot)
				r.Delete("/vm-snapshots/{namespace}/{name}", vmsH.DeleteSnapshot)
				r.Post("/vm-restores", vmsH.CreateRestore)
				r.Delete("/vm-restores/{namespace}/{name}", vmsH.DeleteRestore)
			})
		}

		// ---- Workloads (Apps) — Helm-driven Package Center backend ----
		// Always mounted; when WorkloadsMgr is nil the handlers respond 503
		// so the GUI gets a stable shape on hosts where k3s isn't ready.
		{
			workloadsH := &handlers.WorkloadsHandler{Logger: d.Logger, Lifecycle: d.WorkloadsMgr}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermWorkloadsRead))
				r.Get("/workloads/index", workloadsH.ListIndex)
				r.Get("/workloads/index/{name}", workloadsH.GetIndexEntry)
				r.Get("/workloads", workloadsH.List)
				r.Get("/workloads/{releaseName}", workloadsH.Get)
				r.Get("/workloads/{releaseName}/events", workloadsH.Events)
				r.Get("/workloads/{releaseName}/logs", workloadsH.Logs)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermWorkloadsWrite))
				r.Post("/workloads/index/reload", workloadsH.ReloadIndex)
				r.Post("/workloads", workloadsH.Install)
				r.Patch("/workloads/{releaseName}", workloadsH.Upgrade)
				r.Delete("/workloads/{releaseName}", workloadsH.Uninstall)
				r.Post("/workloads/{releaseName}/rollback", workloadsH.Rollback)
			})
		}

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

			// Scrub policies. Read covers list/get; write covers CRUD.
			// The ad-hoc /api/v1/pools/{name}/scrub trigger is owned by
			// PoolsWriteHandler.Scrub above (storage:write); this group
			// is purely the policy resource. PermScrubRead/PermScrubWrite
			// are also granted to nova-operator so on-call engineers can
			// edit non-builtin policies without admin rights.
			scrubH := &handlers.ScrubPolicyHandler{Logger: d.Logger, Q: d.Store.Queries, Dispatcher: d.Dispatcher}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermScrubRead))
				r.Get("/scrub-policies", scrubH.List)
				r.Get("/scrub-policies/{id}", scrubH.Get)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermScrubWrite))
				r.Post("/scrub-policies", scrubH.Create)
				r.Patch("/scrub-policies/{id}", scrubH.Update)
				r.Delete("/scrub-policies/{id}", scrubH.Delete)
			})

			// Replication jobs. Read endpoints are gated on
			// PermReplicationRead; create/update/delete and the ad-hoc
			// /run trigger require PermReplicationWrite. The handler is
			// only mounted when ReplicationMgr is wired (production path).
			if d.ReplicationMgr != nil {
				replH := &handlers.ReplicationHandler{
					Logger:     d.Logger,
					Mgr:        d.ReplicationMgr,
					Dispatcher: d.Dispatcher,
					Secrets:    nil,
				}
				if d.Secrets != nil {
					replH.Secrets = &replicationSecretsCleaner{m: d.Secrets}
				}
				r.Group(func(r chi.Router) {
					r.Use(require(auth.PermReplicationRead))
					r.Get("/replication-jobs", replH.List)
					r.Get("/replication-jobs/{id}", replH.Get)
					r.Get("/replication-jobs/{id}/runs", replH.Runs)
				})
				r.Group(func(r chi.Router) {
					r.Use(require(auth.PermReplicationWrite))
					r.Post("/replication-jobs", replH.Create)
					r.Patch("/replication-jobs/{id}", replH.Update)
					r.Delete("/replication-jobs/{id}", replH.Delete)
					r.Post("/replication-jobs/{id}/run", replH.Run)
				})
			}

			// Audit read & export. Gated on PermAuditRead — operator+
			// only. nova-viewer intentionally lacks this permission so
			// reading the audit log doesn't itself become a stealthy
			// reconnaissance vector.
			auditH := &handlers.AuditReadHandler{Logger: d.Logger, Q: d.Store.Queries}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermAuditRead))
				r.Get("/audit", auditH.List)
				r.Get("/audit/summary", auditH.Summary)
				r.Get("/audit/export", auditH.Export)
			})
		}

		// ---- Alerts (Alertmanager pass-through) ----
		if d.AlertsHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermAlertsRead))
				r.Get("/alerts", d.AlertsHandler.ListAlerts)
				r.Get("/alerts/{fingerprint}", d.AlertsHandler.GetAlert)
				r.Get("/alert-silences", d.AlertsHandler.ListSilences)
				r.Get("/alert-receivers", d.AlertsHandler.ListReceivers)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermAlertsWrite))
				r.Post("/alert-silences", d.AlertsHandler.CreateSilence)
				r.Delete("/alert-silences/{id}", d.AlertsHandler.DeleteSilence)
			})
		}

		// ---- Logs (Loki pass-through) ----
		if d.LogsHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermLogsRead))
				r.Get("/logs/query", d.LogsHandler.QueryRange)
				r.Get("/logs/query/instant", d.LogsHandler.QueryInstant)
				r.Get("/logs/labels", d.LogsHandler.Labels)
				r.Get("/logs/labels/{name}/values", d.LogsHandler.LabelValues)
				r.Get("/logs/series", d.LogsHandler.Series)
				r.Get("/logs/tail", d.LogsHandler.Tail)
			})
		}

		// ---- Sessions / login history (Keycloak admin pass-through) ----
		if d.SessionsHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermSessionsRead))
				r.Get("/auth/sessions", d.SessionsHandler.ListOwnSessions)
				r.Delete("/auth/sessions/{id}", d.SessionsHandler.RevokeOwnSession)
				r.Get("/auth/login-history", d.SessionsHandler.ListOwnLoginHistory)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermSessionsAdmin))
				r.Get("/auth/users/{id}/sessions", d.SessionsHandler.ListUserSessions)
				r.Delete("/auth/users/{id}/sessions", d.SessionsHandler.RevokeUserSessions)
				r.Get("/auth/users/{id}/login-history", d.SessionsHandler.ListUserLoginHistory)
			})
		}

		// ---- Tier 2 plugins (first-party marketplace + lifecycle) ----
		// Always mounted; when PluginsMgr is nil every endpoint responds
		// 503 so the Aurora chrome gets a stable shape on hosts where
		// the engine has not been wired yet (e.g. dev VMs).
		{
			pluginsH := &handlers.PluginsHandler{
				Logger:      d.Logger,
				Manager:     d.PluginsMgr,
				Marketplace: d.PluginsMarket,
				Router:      d.PluginsRouter,
				UI:          d.PluginsUI,
			}
			// Preview handler is logically distinct from PluginsHandler:
			// it needs only the marketplace + verifier, not the engine
			// Manager. We borrow Verifier from the Manager so wiring
			// stays single-source.
			var previewVerifier *plugins.Verifier
			if d.PluginsMgr != nil {
				previewVerifier = d.PluginsMgr.Verifier
			}
			var previewAuditor handlers.PluginsPreviewAuditor
			if d.Store != nil {
				previewAuditor = d.Store.Queries
			}
			pluginsPreviewH := &handlers.PluginsPreviewHandler{
				Logger:      d.Logger,
				Marketplace: d.PluginsMarket,
				Verifier:    previewVerifier,
				Auditor:     previewAuditor,
			}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermPluginsRead))
				r.Get("/plugins/index", pluginsH.Index)
				r.Get("/plugins/categories", pluginsH.Categories)
				r.Get("/plugins/index/{name}", pluginsH.IndexEntry)
				r.Get("/plugins/index/{name}/manifest", pluginsPreviewH.Preview)
				r.Get("/plugins", pluginsH.List)
				r.Get("/plugins/{name}", pluginsH.Get)
				r.Get("/plugins/{name}/dependencies", pluginsH.Dependencies)
				r.Get("/plugins/{name}/dependents", pluginsH.Dependents)
				// Static UI bundle. Open to any read-capable identity so
				// Aurora can lazy-load the React module without escalating.
				r.Get("/plugins/{name}/ui/*", pluginsH.ServeUI)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermPluginsAdmin))
				r.Post("/plugins", pluginsH.Install)
				r.Patch("/plugins/{name}", pluginsH.Upgrade)
				r.Delete("/plugins/{name}", pluginsH.Uninstall)
			})
			// Reverse-proxy: /plugins/{name}/api/* dispatches into the
			// installed plugin's manifest-declared upstreams. Auth is
			// enforced per-route by the manifest (bearer-passthrough vs
			// service-token) inside the router; nova-api still requires
			// PluginsRead so unauthenticated traffic never reaches the
			// upstream.
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermPluginsRead))
				r.HandleFunc("/plugins/{name}/api/*", pluginsH.ServeProxy)
			})
		}

		// ---- Marketplaces registry (multi-source) ----
		// The locked novanas-official entry is seeded at boot. Operators
		// add other marketplaces (TrueCharts, third-party publishers,
		// internal mirrors) via POST. Adding a marketplace expands the
		// trust surface, so write paths are admin-gated.
		{
			marketsH := &handlers.MarketplacesHandler{
				Logger: d.Logger,
				Store:  d.MarketplacesStore,
				Multi:  d.MarketplacesMulti,
				AddedBy: func(req *http.Request) string {
					if id, ok := auth.IdentityFromContext(req.Context()); ok && id != nil {
						return id.Subject
					}
					return ""
				},
			}
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermMarketplacesRead))
				r.Get("/marketplaces", marketsH.List)
				r.Get("/marketplaces/{id}", marketsH.Get)
			})
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermMarketplacesAdmin))
				r.Post("/marketplaces", marketsH.Create)
				r.Patch("/marketplaces/{id}", marketsH.Patch)
				r.Delete("/marketplaces/{id}", marketsH.Delete)
				r.Post("/marketplaces/{id}/refresh-trust-key", marketsH.RefreshTrustKey)
			})
		}

		// ---- System version / updates ----
		if d.SystemMetaHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(require(auth.PermSystemRead))
				r.Get("/system/version", d.SystemMetaHandler.GetVersion)
				r.Get("/system/updates", d.SystemMetaHandler.GetUpdates)
			})
		}
	})

	return &Server{deps: d, router: r}
}

func (s *Server) Handler() http.Handler { return s.router }

// replicationSecretsCleaner adapts secrets.Manager to the
// handlers.ReplicationSecretsCleaner interface so the handler can
// purge per-job secrets on DELETE without a direct dep on the secrets
// package.
type replicationSecretsCleaner struct {
	m secrets.Manager
}

func (c *replicationSecretsCleaner) DeleteJobSecrets(ctx context.Context, id uuid.UUID) error {
	return replication.DeleteJobSecrets(ctx, c.m, id)
}

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
