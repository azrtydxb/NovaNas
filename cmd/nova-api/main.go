// Command nova-api is the storage control plane HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api"
	"github.com/novanas/nova-nas/internal/api/metrics"
	"github.com/novanas/nova-nas/internal/auth"
	"github.com/novanas/nova-nas/internal/config"
	"github.com/novanas/nova-nas/internal/host/disks"
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
	"github.com/novanas/nova-nas/internal/host/tpm"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/logging"
	"github.com/novanas/nova-nas/internal/replication"
	"github.com/novanas/nova-nas/internal/scrubpolicy"
	"github.com/novanas/nova-nas/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	logger, err := logging.New(cfg.LogLevel, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logger:", err)
		os.Exit(1)
	}

	// Register the TPM-backed sealer factory before secrets.FromEnv so
	// SECRETS_FILE_TPM_SEAL=true can resolve a real Sealer. The factory
	// is invoked lazily and only if the env opts in.
	secrets.RegisterTPMSealerFactory(func() (secrets.Sealer, error) {
		return tpm.New(logger)
	})

	// Build the secret-storage manager from env. Failure here is fatal:
	// later code may need to read DB or Redis credentials at startup.
	secretsMgr, err := secrets.FromEnv(logger)
	if err != nil {
		logger.Error("secrets init", "err", err)
		os.Exit(1)
	}
	logger.Info("secrets backend ready", "backend", secretsMgr.Backend())

	// Build the OIDC verifier. When OIDC_DISABLED=true we skip verifier
	// construction entirely and emit a loud WARN so operators notice if
	// dev mode escapes a local box.
	var verifier *auth.Verifier
	if cfg.Auth.Disabled {
		logger.Warn("OIDC AUTH DISABLED — all /api/v1 routes are publicly reachable; set OIDC_DISABLED=false to enable verification")
	} else {
		auth.SetDevLogger(logger)
		verifier, err = auth.NewVerifier(auth.Config{
			IssuerURL:          cfg.Auth.IssuerURL,
			Audience:           cfg.Auth.Audience,
			RequiredRolePrefix: cfg.Auth.RequiredRolePrefix,
			ResourceClient:     cfg.Auth.ClientID,
		}, nil)
		if err != nil {
			logger.Error("auth init", "err", err)
			os.Exit(1)
		}
	}

	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	disksLister := &disks.Lister{LsblkBin: cfg.LsblkBin}
	poolMgr := &pool.Manager{ZpoolBin: cfg.ZpoolBin}
	datasetMgr := &dataset.Manager{ZFSBin: cfg.ZFSBin}
	snapMgr := &snapshot.Manager{ZFSBin: cfg.ZFSBin}
	iscsiMgr := &iscsi.Manager{}
	nvmeofMgr := &nvmeof.Manager{}
	nfsMgr := &nfs.Manager{RequireKerberos: cfg.NFSRequireKerberos}
	krb5Mgr := &krb5.Manager{}
	var krb5KDC *krb5.KDCManager
	if cfg.Krb5KDCEnabled {
		krb5KDC = &krb5.KDCManager{Cfg: krb5.KDCConfig{Realm: cfg.Krb5Realm}}
	}
	rdmaLister := &rdma.Lister{}
	sambaMgr := &samba.Manager{}
	smartMgr := &smart.Manager{}
	networkMgr := &network.Manager{}
	if _, err := os.Stat("/usr/sbin/ip"); err != nil {
		logger.Warn("network: /usr/sbin/ip not found; live interface listing will fail",
			"err", err)
	}
	systemMgr := &system.Manager{}
	psMgr := protocolshare.New(datasetMgr, nfsMgr, sambaMgr)

	// SMTP relay manager. Built unconditionally so the API can rotate
	// the config at runtime even on hosts that started without one
	// configured. When SMTP_HOST is empty Send/SendTest return
	// ErrNotConfigured until PUT /api/v1/notifications/smtp populates it.
	smtpCfg := notifysmtp.Config{
		Host:        cfg.SMTP.Host,
		Port:        cfg.SMTP.Port,
		Username:    cfg.SMTP.Username,
		FromAddress: cfg.SMTP.From,
		TLSMode:     notifysmtp.TLSMode(cfg.SMTP.TLSMode),
	}
	if cfg.SMTP.PasswordFile != "" {
		b, err := os.ReadFile(cfg.SMTP.PasswordFile)
		if err != nil {
			logger.Warn("smtp password file unreadable", "path", cfg.SMTP.PasswordFile, "err", err)
		} else {
			// Strip a single trailing newline; operators routinely produce
			// these with `echo … > file`.
			s := string(b)
			for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
				s = s[:len(s)-1]
			}
			smtpCfg.Password = s
		}
	}
	smtpMgr, err := notifysmtp.NewManager(smtpCfg, cfg.SMTP.MaxPerMinute)
	if err != nil {
		// Bad config (e.g. SMTP_HOST set but SMTP_FROM empty). Log and
		// fall back to an unconfigured manager so the API still starts —
		// the operator can fix it via PUT.
		logger.Warn("smtp init", "err", err)
		smtpMgr, _ = notifysmtp.NewManager(notifysmtp.Config{}, cfg.SMTP.MaxPerMinute)
	}
	schedulerMgr := scheduler.New(logger, st.Queries, snapMgr, datasetMgr, nil)

	// Asynq client (dispatcher uses this)
	asynqRedisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "redis parse:", err)
		os.Exit(1)
	}
	asyncClient := asynq.NewClient(asynqRedisOpt)
	defer asyncClient.Close()

	// Plain redis client (for SSE pub/sub)
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "redis url:", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	// Metrics registry. Owned by main so the same handle can be wired
	// into the dispatcher, worker, and HTTP server, plus the optional
	// separate listener for /metrics.
	metricsReg := metrics.New()
	zfsCollector := metrics.NewZFSCollector(logger, poolMgr, datasetMgr)
	zfsCollector.MustRegister(metricsReg.Registry)

	dispatcher := &jobs.Dispatcher{
		Client:  asyncClient,
		Queries: st.Queries,
		Pool:    st.Pool,
		Metrics: metricsReg.Jobs,
	}

	// Crash recovery: mark any running/queued rows as interrupted before
	// the worker starts consuming new tasks.
	if err := jobs.MarkInterruptedAtStartup(ctx, st.Queries); err != nil {
		logger.Error("recovery", "err", err)
	}

	// Replication subsystem (general, supersedes the scheduler-driven
	// path for new jobs). Backends with no concrete deps wired today
	// validate jobs and reject runs at execute-time; the worker handler
	// resolves credentials per run via the secrets manager.
	replStore := replication.NewPgxStore(st.Queries)
	replMgr := replication.NewManager(
		replStore,
		replication.NewMemLocker(),
		[]replication.Backend{
			&replication.ZFSBackend{},
			&replication.S3Backend{},
			&replication.RsyncBackend{},
		},
		replication.ManagerOptions{Logger: logger},
	)

	asyncSrv := asynq.NewServer(asynqRedisOpt, asynq.Config{
		Concurrency: 4,
		Logger:      asynqSlogAdapter{l: logger},
	})
	mux := jobs.NewServeMux(jobs.WorkerDeps{
		Logger:       logger,
		Queries:      st.Queries,
		Redis:        redisClient,
		Secrets:      secretsMgr,
		Pools:        poolMgr,
		Datasets:     datasetMgr,
		Snapshots:    snapMgr,
		IscsiMgr:     iscsiMgr,
		NvmeofMgr:    nvmeofMgr,
		NfsMgr:       nfsMgr,
		Krb5Mgr:      krb5Mgr,
		SambaMgr:     sambaMgr,
		SmartMgr:     smartMgr,
		SchedulerMgr: schedulerMgr,
		NetworkMgr:       networkMgr,
		SystemMgr:        systemMgr,
		ProtocolShareMgr: psMgr,
		Metrics:          metricsReg.Jobs,
	})
	// Register the new general replication-run handler. Coexists with
	// the older scheduler.replication.fire kind in NewServeMux for
	// backward compat.
	mux.Handle(string(jobs.KindReplicationRun), jobs.HandleReplicationRun(replMgr))

	go func() {
		if err := asyncSrv.Run(mux); err != nil {
			// Mirror the HTTP listener: a dead worker is a hard failure.
			// Without this, HTTP keeps accepting writes that enqueue
			// jobs nothing will ever execute.
			logger.Error("asynq run", "err", err)
			os.Exit(1)
		}
	}()
	defer asyncSrv.Stop()

	// ZFS metrics poll loop. Cancelled alongside the HTTP server on
	// shutdown via metricsCtx.
	metricsCtx, metricsCancel := context.WithCancel(context.Background())
	defer metricsCancel()
	go zfsCollector.Run(metricsCtx)

	// Scheduler tick loop. Cancelled alongside the HTTP server on
	// shutdown via schedCtx. Errors from Run are only ctx.Err() once
	// shutdown begins; logged at debug.
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	go func() {
		if err := schedulerMgr.Run(schedCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scheduler run", "err", err)
		}
	}()

	// Scrub-policy manager. Bootstraps the operator-default policy on a
	// fresh install (idempotent — re-running install or restarts never
	// duplicate the row) then starts its own tick loop that dispatches
	// KindPoolScrub jobs when policies are due. Runs alongside the
	// snapshot/replication scheduler; same cancellation semantics.
	scrubMgr := scrubpolicy.New(logger, st.Queries, poolMgr, dispatcher)
	if _, err := scrubMgr.EnsureDefaultPolicy(ctx); err != nil {
		logger.Error("scrubpolicy bootstrap", "err", err)
	}
	scrubCtx, scrubCancel := context.WithCancel(context.Background())
	defer scrubCancel()
	go func() {
		if err := scrubMgr.Run(scrubCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scrubpolicy run", "err", err)
		}
	}()

	// Start the cron-tick loop for replication jobs. We re-use the same
	// cron parser the snapshot scheduler uses (internal/host/scheduler)
	// and dispatch via the existing jobs.Dispatcher so /jobs metadata
	// stays uniform across kinds.
	replCtx, replCancel := context.WithCancel(context.Background())
	defer replCancel()
	go runReplicationScheduler(replCtx, logger, replStore, dispatcher)

	srv := api.New(api.Deps{
		Logger:     logger,
		Store:      st,
		Disks:      disksLister,
		Pools:      poolMgr,
		Datasets:   datasetMgr,
		Snapshots:  snapMgr,
		Dispatcher: dispatcher,
		Redis:      redisClient,
		DatasetMgr:  datasetMgr,
		PoolMgr:     poolMgr,
		SnapshotMgr: snapMgr,
		IscsiMgr:    iscsiMgr,
		NvmeofMgr:   nvmeofMgr,
		NfsMgr:      nfsMgr,
		Krb5Mgr:     krb5Mgr,
		Krb5KDC:     krb5KDC,
		RdmaLister:  rdmaLister,
		SambaMgr:     sambaMgr,
		SmartMgr:     smartMgr,
		SchedulerMgr: schedulerMgr,
		NetworkMgr:       networkMgr,
		SystemMgr:        systemMgr,
		ProtocolShareMgr: psMgr,
		SMTPMgr:          smtpMgr,
		EncryptionMgr:    buildEncryptionMgr(logger, datasetMgr, secretsMgr),
		ReplicationMgr:   replMgr,

		Verifier:     verifier,
		RoleMap:      auth.DefaultRoleMap,
		AuthDisabled: cfg.Auth.Disabled,
		Secrets:      secretsMgr,

		Metrics:        metricsReg,
		MetricsHandler: metricsHandlerFor(cfg.MetricsAddr, metricsReg),
	})

	// Optional dedicated listener for /metrics. When METRICS_ADDR is set
	// the main API listener does NOT expose /metrics (MetricsHandler is
	// nil there); a small mux is bound on the separate address instead so
	// Prometheus can scrape it from the management network.
	if cfg.MetricsAddr != "" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metricsReg.Handler())
		metricsSrv := &http.Server{
			Addr:              cfg.MetricsAddr,
			Handler:           metricsMux,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		logger.Info("metrics listener", "addr", cfg.MetricsAddr)
		go func() {
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics listen", "err", err)
			}
		}()
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsSrv.Shutdown(ctx)
		}()
	}
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout is intentionally 0: SSE on /api/v1/jobs/{id}/stream
		// holds the connection open indefinitely while pushing state
		// updates. A non-zero WriteTimeout would terminate the stream.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	tlsCancel, err := startTLS(context.Background(), cfg.TLS, logger, srv.Handler())
	if err != nil {
		logger.Error("tls start", "err", err)
		os.Exit(1)
	}
	defer tlsCancel()

	logger.Info("starting", "addr", cfg.ListenAddr)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http listen", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	logger.Info("stopped")
}

// metricsHandlerFor returns the handler that the main API server should
// mount at /metrics. When METRICS_ADDR is set we deliberately return nil
// so the main listener does NOT serve /metrics — the dedicated listener
// owns the endpoint exclusively (separate-listener model). Otherwise the
// promhttp handler is mounted on the main listener.
func metricsHandlerFor(metricsAddr string, m *metrics.Metrics) http.Handler {
	if metricsAddr != "" {
		return nil
	}
	return m.Handler()
}

// buildEncryptionMgr constructs the ZFS native-encryption manager
// (TPM-sealed key escrow) wired to the host TPM and the runtime
// secrets backend.
//
// If the host has no TPM (e.g. in CI / VM-without-vTPM) we return
// nil; the API surface degrades gracefully — encryption endpoints
// respond 503 — without crashing the server.
func buildEncryptionMgr(logger *slog.Logger, dsMgr *dataset.Manager, secretsMgr secrets.Manager) *dataset.EncryptionManager {
	sealer, err := tpm.New(logger)
	if err != nil {
		logger.Warn("encryption manager: TPM unavailable; /encryption endpoints will return 503",
			"err", err)
		return nil
	}
	zfsBin := "/sbin/zfs"
	if dsMgr != nil && dsMgr.ZFSBin != "" {
		zfsBin = dsMgr.ZFSBin
	}
	return &dataset.EncryptionManager{
		ZFSBin:  zfsBin,
		Sealer:  sealer,
		Secrets: secretsMgr,
	}
}

// runReplicationScheduler is the cron tick loop for replication jobs.
// On every tick it lists enabled jobs, evaluates each job's cron
// expression against (last_fired_at, now] using the existing scheduler
// cron parser, and dispatches an Asynq replication.run task per match.
//
// It runs entirely off the database so the cron registration is
// idempotent across restarts: the act of having a row with a non-empty
// schedule is the registration. There is no in-memory cron table to
// drift from disk.
func runReplicationScheduler(ctx context.Context, logger *slog.Logger, store *replication.PgxStore, d *jobs.Dispatcher) {
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()
	loop := func() {
		now := time.Now().UTC()
		js, err := store.ListEnabledJobs(ctx)
		if err != nil {
			logger.Error("replication scheduler: list enabled", "err", err)
			return
		}
		for _, j := range js {
			if j.Schedule == "" {
				continue
			}
			expr, err := scheduler.ParseCron(j.Schedule)
			if err != nil {
				logger.Warn("replication scheduler: bad cron, skipping", "id", j.ID.String(), "cron", j.Schedule, "err", err)
				continue
			}
			// We don't have last_fired_at on replication.Job today; use
			// updated_at as the lower bound. This means a freshly-edited
			// job won't double-fire on the minute it was edited; that
			// trade-off is acceptable for v1.
			prev := j.UpdatedAt
			if !expr.ShouldFireBetween(prev, now, time.UTC) {
				continue
			}
			if _, derr := jobs.DispatchReplication(ctx, d, j.ID, "scheduler:tick", "replication-scheduler"); derr != nil {
				if errors.Is(derr, jobs.ErrDuplicate) {
					continue
				}
				logger.Error("replication scheduler: dispatch failed", "id", j.ID.String(), "err", derr)
				continue
			}
			pgts := pgtype.Timestamptz{Time: now, Valid: true}
			if err := store.MarkFired(ctx, j.ID, pgts); err != nil {
				logger.Warn("replication scheduler: mark fired", "id", j.ID.String(), "err", err)
			}
		}
	}
	loop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			loop()
		}
	}
}

type asynqSlogAdapter struct{ l *slog.Logger }

func (a asynqSlogAdapter) Debug(args ...any) { a.l.Debug("asynq", "args", args) }
func (a asynqSlogAdapter) Info(args ...any)  { a.l.Info("asynq", "args", args) }
func (a asynqSlogAdapter) Warn(args ...any)  { a.l.Warn("asynq", "args", args) }
func (a asynqSlogAdapter) Error(args ...any) { a.l.Error("asynq", "args", args) }
func (a asynqSlogAdapter) Fatal(args ...any) { a.l.Error("asynq", "args", args) }
