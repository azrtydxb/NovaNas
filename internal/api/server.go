// Package api wires the HTTP server.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api/handlers"
	mw "github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/iscsi"
	"github.com/novanas/nova-nas/internal/host/krb5"
	"github.com/novanas/nova-nas/internal/host/network"
	"github.com/novanas/nova-nas/internal/host/nfs"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
	"github.com/novanas/nova-nas/internal/host/protocolshare"
	"github.com/novanas/nova-nas/internal/host/rdma"
	"github.com/novanas/nova-nas/internal/host/samba"
	"github.com/novanas/nova-nas/internal/host/scheduler"
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
	RdmaLister    *rdma.Lister
	SambaMgr      *samba.Manager
	SmartMgr      *smart.Manager
	SchedulerMgr  *scheduler.Manager
	NetworkMgr    *network.Manager
	SystemMgr     *system.Manager
	ProtocolShareMgr *protocolshare.Manager
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
		r.Post("/pools/{name}/checkpoint", poolsWriteH.Checkpoint)
		r.Post("/pools/{name}/discard-checkpoint", poolsWriteH.DiscardCheckpoint)
		r.Post("/pools/{name}/upgrade", poolsWriteH.Upgrade)
		r.Post("/pools/{name}/reguid", poolsWriteH.Reguid)
		r.Post("/pools/sync", poolSync.Sync)
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
		r.Post("/datasets/{fullname}/bookmark", dsW.Bookmark)
		r.Post("/datasets/{fullname}/destroy-bookmark", dsW.DestroyBookmark)
		r.Get("/datasets/{fullname}/diff", dsQuery.Diff)
		r.Get("/datasets/{fullname}/bookmarks", dsQuery.ListBookmarks)
		r.Get("/snapshots", snapH.List)
		r.Post("/snapshots", snapW.Create)
		r.Delete("/snapshots/{fullname}", snapW.Destroy)
		r.Post("/datasets/{fullname}/rollback", snapW.Rollback)
		r.Post("/snapshots/{fullname}/hold", snapW.Hold)
		r.Post("/snapshots/{fullname}/release", snapW.Release)
		r.Get("/snapshots/{fullname}/holds", snapHolds.Holds)

		// iSCSI
		if iscsiH != nil {
			r.Get("/iscsi/targets", iscsiH.ListTargets)
			r.Get("/iscsi/targets/{iqn}", iscsiH.GetTarget)
		}
		r.Post("/iscsi/targets", iscsiW.CreateTarget)
		r.Delete("/iscsi/targets/{iqn}", iscsiW.DestroyTarget)
		r.Post("/iscsi/targets/{iqn}/portals", iscsiW.CreatePortal)
		r.Delete("/iscsi/targets/{iqn}/portals/{ip}/{port}", iscsiW.DeletePortal)
		r.Post("/iscsi/targets/{iqn}/luns", iscsiW.CreateLUN)
		r.Delete("/iscsi/targets/{iqn}/luns/{id}", iscsiW.DeleteLUN)
		r.Post("/iscsi/targets/{iqn}/acls", iscsiW.CreateACL)
		r.Delete("/iscsi/targets/{iqn}/acls/{initiatorIqn}", iscsiW.DeleteACL)
		r.Post("/iscsi/saveconfig", iscsiW.SaveConfig)

		// NVMe-oF
		if nvmeofH != nil {
			r.Get("/nvmeof/subsystems", nvmeofH.ListSubsystems)
			r.Get("/nvmeof/subsystems/{nqn}", nvmeofH.GetSubsystem)
			r.Get("/nvmeof/ports", nvmeofH.ListPorts)
			r.Get("/nvmeof/hosts/{nqn}/dhchap", nvmeofH.GetHostDHChap)
		}
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

		// NFS
		if nfsH != nil {
			r.Get("/nfs/exports", nfsH.ListExports)
			r.Get("/nfs/exports/active", nfsH.ListActive)
			r.Get("/nfs/exports/{name}", nfsH.GetExport)
		}
		r.Post("/nfs/exports", nfsW.CreateExport)
		r.Patch("/nfs/exports/{name}", nfsW.UpdateExport)
		r.Delete("/nfs/exports/{name}", nfsW.DeleteExport)
		r.Post("/nfs/reload", nfsW.Reload)

		// Kerberos
		if krb5H != nil {
			r.Get("/krb5/config", krb5H.GetConfig)
			r.Get("/krb5/idmapd", krb5H.GetIdmapd)
			r.Get("/krb5/keytab", krb5H.ListKeytab)
		}
		r.Put("/krb5/config", krb5W.SetConfig)
		r.Put("/krb5/idmapd", krb5W.SetIdmapd)
		r.Put("/krb5/keytab", krb5W.UploadKeytab)
		r.Delete("/krb5/keytab", krb5W.DeleteKeytab)

		// RDMA
		r.Get("/network/rdma", rdmaH.List)

		// Samba
		if sambaH != nil {
			r.Get("/samba/shares", sambaH.ListShares)
			r.Get("/samba/shares/{name}", sambaH.GetShare)
			r.Get("/samba/users", sambaH.ListUsers)
		}
		r.Post("/samba/shares", sambaW.CreateShare)
		r.Patch("/samba/shares/{name}", sambaW.UpdateShare)
		r.Delete("/samba/shares/{name}", sambaW.DeleteShare)
		r.Post("/samba/reload", sambaW.Reload)
		r.Post("/samba/users", sambaW.AddUser)
		r.Delete("/samba/users/{username}", sambaW.DeleteUser)
		r.Put("/samba/users/{username}/password", sambaW.SetUserPassword)

		// Samba globals
		if sambaGlobalsH != nil {
			r.Get("/samba/globals", sambaGlobalsH.Get)
			r.Put("/samba/globals", sambaGlobalsH.Set)
		}

		// ProtocolShare (unified NFS+SMB share abstraction)
		if psH != nil {
			r.Get("/protocol-shares", psH.List)
			r.Get("/protocol-shares/{name}", psH.Get)
		}
		r.Post("/protocol-shares", psW.Create)
		r.Patch("/protocol-shares/{name}", psW.Update)
		r.Delete("/protocol-shares/{name}", psW.Delete)

		// Dataset NFSv4 ACL
		if dsACLH != nil {
			r.Get("/datasets/{fullname}/acl", dsACLH.Get)
			r.Put("/datasets/{fullname}/acl", dsACLH.Set)
			r.Post("/datasets/{fullname}/acl/append", dsACLH.Append)
			r.Delete("/datasets/{fullname}/acl/{index}", dsACLH.Remove)
		}

		// SMART
		if smartH != nil {
			r.Get("/disks/{name}/smart", smartH.Get)
			r.Post("/disks/{name}/smart/test", smartH.RunSelfTest)
			r.Post("/disks/{name}/smart/enable", smartH.Enable)
		}

		// Network (configs + live + write)
		if networkH != nil {
			r.Get("/network/interfaces", networkH.ListInterfaces)
			r.Get("/network/configs", networkH.ListConfigs)
			r.Get("/network/configs/{name}", networkH.GetConfig)
		}
		if networkW != nil {
			r.Post("/network/configs", networkW.ApplyInterface)
			r.Delete("/network/configs/{name}", networkW.DeleteInterface)
			r.Post("/network/vlans", networkW.ApplyVLAN)
			r.Post("/network/bonds", networkW.ApplyBond)
			r.Post("/network/reload", networkW.Reload)
		}

		// System
		if systemH != nil {
			r.Get("/system/info", systemH.GetInfo)
			r.Get("/system/time", systemH.GetTime)
			r.Put("/system/hostname", systemH.SetHostname)
			r.Put("/system/timezone", systemH.SetTimezone)
			r.Put("/system/ntp", systemH.SetNTP)
			r.Post("/system/reboot", systemH.Reboot)
			r.Post("/system/shutdown", systemH.Shutdown)
			r.Post("/system/cancel-shutdown", systemH.CancelShutdown)
		}

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

			schedH := &handlers.SchedulerHandler{Logger: d.Logger, Q: d.Store.Queries}
			r.Get("/scheduler/snapshot-schedules", schedH.ListSnapshotSchedules)
			r.Post("/scheduler/snapshot-schedules", schedH.CreateSnapshotSchedule)
			r.Get("/scheduler/snapshot-schedules/{id}", schedH.GetSnapshotSchedule)
			r.Patch("/scheduler/snapshot-schedules/{id}", schedH.UpdateSnapshotSchedule)
			r.Delete("/scheduler/snapshot-schedules/{id}", schedH.DeleteSnapshotSchedule)
			r.Get("/scheduler/replication-targets", schedH.ListReplicationTargets)
			r.Post("/scheduler/replication-targets", schedH.CreateReplicationTarget)
			r.Get("/scheduler/replication-targets/{id}", schedH.GetReplicationTarget)
			r.Delete("/scheduler/replication-targets/{id}", schedH.DeleteReplicationTarget)
			r.Get("/scheduler/replication-schedules", schedH.ListReplicationSchedules)
			r.Post("/scheduler/replication-schedules", schedH.CreateReplicationSchedule)
			r.Get("/scheduler/replication-schedules/{id}", schedH.GetReplicationSchedule)
			r.Patch("/scheduler/replication-schedules/{id}", schedH.UpdateReplicationSchedule)
			r.Delete("/scheduler/replication-schedules/{id}", schedH.DeleteReplicationSchedule)

			metaH := &handlers.MetadataHandler{Logger: d.Logger, Q: d.Store.Queries}
			r.Patch("/pools/{name}/metadata", metaH.PoolPatch)
			r.Patch("/datasets/{fullname}/metadata", metaH.DatasetPatch)
			r.Patch("/snapshots/{fullname}/metadata", metaH.SnapshotPatch)
		}
	})

	return &Server{deps: d, router: r}
}

func (s *Server) Handler() http.Handler { return s.router }
