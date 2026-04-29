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
	"github.com/novanas/nova-nas/internal/api/handlers"
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
	hostls "github.com/novanas/nova-nas/internal/host/tls"
	"github.com/novanas/nova-nas/internal/host/tpm"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/logging"
	"github.com/novanas/nova-nas/internal/notifycenter"
	"github.com/novanas/nova-nas/internal/plugins"
	"github.com/novanas/nova-nas/internal/replication"
	"github.com/novanas/nova-nas/internal/scrubpolicy"
	"github.com/novanas/nova-nas/internal/store"
	"github.com/novanas/nova-nas/internal/vms"
	"github.com/novanas/nova-nas/internal/workloads"
)

// Build metadata stamped via -ldflags in CI:
//
//	-ldflags "-X main.buildCommit=$(git rev-parse HEAD) -X main.buildTime=$(date -u +%FT%TZ)"
//
// When unset, /system/version falls back to runtime/debug.ReadBuildInfo().
var (
	buildCommit string
	buildTime   string
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
		Logger:           logger,
		Queries:          st.Queries,
		Redis:            redisClient,
		Secrets:          secretsMgr,
		Pools:            poolMgr,
		Datasets:         datasetMgr,
		Snapshots:        snapMgr,
		IscsiMgr:         iscsiMgr,
		NvmeofMgr:        nvmeofMgr,
		NfsMgr:           nfsMgr,
		Krb5Mgr:          krb5Mgr,
		SambaMgr:         sambaMgr,
		SmartMgr:         smartMgr,
		SchedulerMgr:     schedulerMgr,
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

	// Notification Center (the bell). The manager owns the SSE bus
	// and the per-user state DAO; the aggregator goroutine polls
	// Alertmanager, jobs, and the audit log every 30s and pushes new
	// rows into the manager. RecordEvent is idempotent on
	// (source, source_id) so the cursor reset on restart is safe.
	notifyMgr := notifycenter.NewManager(st.Queries, logger)
	notifyAgg := notifycenter.NewAggregator(notifyMgr, st.Queries, notifycenter.AggregatorConfig{
		AlertmanagerURL: cfg.AlertmanagerURL,
	}, logger)
	notifyCtx, notifyCancel := context.WithCancel(context.Background())
	defer notifyCancel()
	go func() {
		if err := notifyAgg.Run(notifyCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("notifycenter aggregator", "err", err)
		}
	}()

	// Pass-through handlers for Alertmanager, Loki, and the Keycloak
	// admin API. Each is opt-in via env: AlertmanagerURL/LokiURL default
	// to loopback addresses; the Keycloak admin client only mounts when
	// KEYCLOAK_ADMIN_CLIENT_ID + a readable secret file are configured.
	alertsHandler := &handlers.AlertsHandler{Logger: logger, UpstreamURL: cfg.AlertmanagerURL}
	logsHandler := &handlers.LogsHandler{Logger: logger, UpstreamURL: cfg.LokiURL}
	systemMetaHandler := &handlers.SystemMetaHandler{Logger: logger, BuildCommit: buildCommit, BuildTime: buildTime}

	var sessionsHandler *handlers.SessionsHandler
	var keycloakAdmin *handlers.KeycloakAdminClient
	if cfg.KeycloakAdminClientID != "" && cfg.KeycloakAdminClientSecretFile != "" {
		secretBytes, err := os.ReadFile(cfg.KeycloakAdminClientSecretFile)
		if err != nil {
			logger.Warn("keycloak admin secret unreadable; sessions endpoints disabled",
				"path", cfg.KeycloakAdminClientSecretFile, "err", err)
		} else {
			secret := string(secretBytes)
			for len(secret) > 0 && (secret[len(secret)-1] == '\n' || secret[len(secret)-1] == '\r') {
				secret = secret[:len(secret)-1]
			}
			adminURL := cfg.KeycloakAdminURL
			tokenURL := ""
			// Derive admin and token URLs from OIDC_ISSUER_URL when not set.
			// Issuer:  https://kc/realms/<realm>
			// Admin:   https://kc/admin/realms/<realm>
			// Token:   https://kc/realms/<realm>/protocol/openid-connect/token
			issuer := cfg.Auth.IssuerURL
			if adminURL == "" && issuer != "" {
				if i := stringIndex(issuer, "/realms/"); i > 0 {
					base := issuer[:i]
					realm := issuer[i+len("/realms/"):]
					adminURL = base + "/admin/realms/" + realm
				}
			}
			if issuer != "" {
				tokenURL = issuer + "/protocol/openid-connect/token"
			}
			if adminURL != "" && tokenURL != "" {
				keycloakAdmin = &handlers.KeycloakAdminClient{
					AdminURL:     adminURL,
					TokenURL:     tokenURL,
					ClientID:     cfg.KeycloakAdminClientID,
					ClientSecret: secret,
				}
				sessionsHandler = &handlers.SessionsHandler{
					Logger:  logger,
					Admin:   keycloakAdmin,
					RoleMap: auth.DefaultRoleMap,
				}
				logger.Info("keycloak admin client wired", "admin_url", adminURL, "client_id", cfg.KeycloakAdminClientID)
			} else {
				logger.Warn("keycloak admin URL or token URL not derivable; sessions endpoints disabled",
					"issuer", issuer, "admin_url", cfg.KeycloakAdminURL)
			}
		}
	}

	// Workloads (Apps) — Helm-driven Package Center backend on the embedded
	// k3s cluster. nova-api runs on the host so we read k3s.yaml from
	// /etc/rancher/k3s/k3s.yaml; if the file is missing or the cluster is
	// unreachable, the manager runs in degraded mode and the /workloads/*
	// endpoints respond 503 — letting nova-api stay up before k3s has been
	// bootstrapped.
	workloadsKubeconfig := os.Getenv("WORKLOADS_KUBECONFIG")
	if workloadsKubeconfig == "" {
		workloadsKubeconfig = "/etc/rancher/k3s/k3s.yaml"
	}
	workloadsIndexPath := os.Getenv("WORKLOADS_INDEX_PATH")
	if workloadsIndexPath == "" {
		workloadsIndexPath = "/usr/share/nova-nas/workloads/index.json"
	}
	workloadsMgr := buildWorkloadsManager(logger, workloadsKubeconfig, workloadsIndexPath)

	// VM management (KubeVirt). Templates are loaded from disk; the
	// KubeClient itself is left nil here — the production wiring is the
	// dynamic-client path scaffolded in internal/vms but the typed
	// kubevirt.io/client-go drop-in is intentionally deferred to a
	// follow-up, so this branch logs a warning and the manager-less API
	// surface returns 503 until the client is wired.
	vmMgr := buildVMManager(logger)

	// Tier 2 plugin engine wiring. Marketplace client is always built
	// (the index URL is configurable). The verifier is wired with the
	// operator-supplied trust key. The provisioner is composed from
	// four sub-provisioners — dataset (ZFS), oidcClient (Keycloak admin),
	// tlsCert (local CA), permission (Keycloak realm-role binding) —
	// and falls back to NopProvisioner when no Keycloak admin client is
	// available so installs still succeed for plugins with no `needs:`.
	// The systemd Deployer is wired unconditionally; the deployer only
	// touches plugins whose deployment.type=systemd.
	pluginsMarket := plugins.NewMarketplaceClient(cfg.MarketplaceIndexURL, nil)
	pluginsVerifier := plugins.NewVerifier(cfg.MarketplaceTrustKeyPath)
	pluginsVerifier.CosignBin = cfg.MarketplaceCosignBin
	pluginsRouter := plugins.NewRouter(logger, nil)
	pluginsUI := plugins.NewUIAssets(cfg.PluginsRoot)
	pluginsProv := buildPluginsProvisioner(logger, datasetMgr, keycloakAdmin, secretsMgr, cfg)
	pluginsDeployer := plugins.NewSystemdDeployer(cfg.PluginsRoot, logger)
	if cfg.PluginsSystemctlBin != "" {
		// Override the default systemctl path. Keep the runner type
		// internal — main.go only knows the bin path.
		pluginsDeployer.Runner = &plugins.SystemctlExec{Bin: cfg.PluginsSystemctlBin}
	}
	pluginsMgr := plugins.NewManager(plugins.ManagerOptions{
		Logger:      logger,
		Queries:     st.Queries,
		Marketplace: pluginsMarket,
		Verifier:    pluginsVerifier,
		Provisioner: pluginsProv,
		Router:      pluginsRouter,
		UI:          pluginsUI,
		Deployer:    pluginsDeployer,
	})
	go pluginsMgr.RestoreAtStartup(context.Background())

	srv := api.New(api.Deps{
		Logger:           logger,
		Store:            st,
		Disks:            disksLister,
		Pools:            poolMgr,
		Datasets:         datasetMgr,
		Snapshots:        snapMgr,
		Dispatcher:       dispatcher,
		Redis:            redisClient,
		DatasetMgr:       datasetMgr,
		PoolMgr:          poolMgr,
		SnapshotMgr:      snapMgr,
		IscsiMgr:         iscsiMgr,
		NvmeofMgr:        nvmeofMgr,
		NfsMgr:           nfsMgr,
		Krb5Mgr:          krb5Mgr,
		Krb5KDC:          krb5KDC,
		RdmaLister:       rdmaLister,
		SambaMgr:         sambaMgr,
		SmartMgr:         smartMgr,
		SchedulerMgr:     schedulerMgr,
		NetworkMgr:       networkMgr,
		SystemMgr:        systemMgr,
		ProtocolShareMgr: psMgr,
		SMTPMgr:          smtpMgr,
		NotifyCenter:     notifyMgr,
		EncryptionMgr:    buildEncryptionMgr(logger, datasetMgr, secretsMgr),
		ReplicationMgr:   replMgr,
		WorkloadsMgr:     workloadsMgr,
		VMMgr:            vmMgr,

		PluginsMgr:    pluginsMgr,
		PluginsRouter: pluginsRouter,
		PluginsUI:     pluginsUI,
		PluginsMarket: pluginsMarket,

		Verifier:     verifier,
		RoleMap:      auth.DefaultRoleMap,
		AuthDisabled: cfg.Auth.Disabled,
		Secrets:      secretsMgr,

		AlertsHandler:     alertsHandler,
		LogsHandler:       logsHandler,
		SessionsHandler:   sessionsHandler,
		SystemMetaHandler: systemMetaHandler,

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

// stringIndex is a tiny strings.Index alias used to keep the import
// surface clean in this file.
func stringIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// buildWorkloadsManager wires the Helm-driven Apps lifecycle manager.
// On a NAS where k3s has not yet been bootstrapped (or is running on a
// non-default kubeconfig path) we still return a manager — the helm
// client itself runs in degraded mode and the API surface responds 503
// for cluster-touching calls until k3s comes up.
func buildWorkloadsManager(logger *slog.Logger, kubeconfigPath, indexPath string) workloads.Lifecycle {
	idx := workloads.NewFileIndex(indexPath)
	if err := idx.Reload(context.Background()); err != nil {
		logger.Warn("workloads: index reload failed; /workloads/index will be empty until reload",
			"path", indexPath, "err", err)
	} else {
		logger.Info("workloads: index loaded", "path", indexPath)
	}
	helm, err := workloads.NewHelmClient(logger, kubeconfigPath)
	if err != nil {
		logger.Warn("workloads: helm client init failed; /workloads endpoints will return 503",
			"err", err)
		return nil
	}
	mgr, err := workloads.NewManager(workloads.ManagerOptions{
		Logger:    logger,
		Index:     idx,
		Helm:      helm,
		IndexPath: indexPath,
	})
	if err != nil {
		logger.Error("workloads: manager init", "err", err)
		return nil
	}
	return mgr
}

// buildPluginsProvisioner composes the production NeedsProvisioner
// from the four sub-provisioners. When the Keycloak admin client is
// not configured (e.g. dev boxes with no realm) the OIDC + Permission
// sub-provisioners are left nil; a plugin claiming those needs will
// fail at install time with a clear error rather than silently
// stubbing them out. Plugins with no `needs:` install fine.
func buildPluginsProvisioner(
	logger *slog.Logger,
	datasetMgr *dataset.Manager,
	keycloakAdmin *handlers.KeycloakAdminClient,
	secretsMgr secrets.Manager,
	cfg *config.Config,
) plugins.NeedsProvisioner {
	dsP := &plugins.DatasetProvisioner{Client: datasetMgr, Logger: logger}
	tlsP := &plugins.TLSCertProvisioner{
		Issuer: &hostls.Issuer{
			CACertPath: cfg.PluginsCACertPath,
			CAKeyPath:  cfg.PluginsCAKeyPath,
		},
		PluginsRoot: cfg.PluginsRoot,
		Logger:      logger,
	}
	var oidcP *plugins.OIDCClientProvisioner
	var permP *plugins.PermissionProvisioner
	if keycloakAdmin != nil {
		oidcP = &plugins.OIDCClientProvisioner{Admin: keycloakAdmin, Secrets: secretsMgr, Logger: logger}
		permP = &plugins.PermissionProvisioner{Admin: keycloakAdmin, Logger: logger}
	} else {
		logger.Warn("plugins: Keycloak admin client unconfigured; oidcClient/permission needs will fail until KEYCLOAK_ADMIN_* is set")
	}
	return plugins.NewProductionProvisioner(dsP, oidcP, tlsP, permP)
}

// buildVMManager wires the KubeVirt-backed VM manager. Templates are
// loaded from VMS_TEMPLATES_PATH (defaults to
// /usr/share/nova-nas/vms/templates.json — operators can override). The
// KubeClient field is intentionally left nil for now; the production
// dynamic-client wiring lands in a follow-up. Until then, the manager
// is wired into Deps but every Kube-touching call no-ops via the 503
// branch in handlers.VMsHandler. The handler explicitly checks for
// Mgr == nil; we set Mgr non-nil only when KubeClient is wired so the
// API correctly returns 503 in single-binary dev installs.
func buildVMManager(logger *slog.Logger) *vms.Manager {
	templatesPath := os.Getenv("VMS_TEMPLATES_PATH")
	if templatesPath == "" {
		templatesPath = "/usr/share/nova-nas/vms/templates.json"
	}
	cat, err := vms.LoadCatalog(templatesPath)
	if err != nil {
		logger.Warn("vms: template catalog unavailable; /vm-templates will be empty",
			"path", templatesPath, "err", err)
		// Empty catalog is fine — the catalog field can be nil and the
		// handler tolerates it.
		cat = nil
	} else {
		logger.Info("vms: template catalog loaded", "path", templatesPath, "count", cat.Count())
	}

	// KubeClient: the dynamic-client implementation lives outside the
	// scope of this commit. When KUBEVIRT_DISABLED is set, or no kubeconfig
	// is reachable, we leave the manager unwired (Deps.VMMgr == nil), and
	// the handler returns 503 for every endpoint.
	if os.Getenv("KUBEVIRT_DISABLED") == "true" {
		logger.Warn("vms: KUBEVIRT_DISABLED=true — /vms endpoints will return 503")
		return nil
	}
	kubeconfig := os.Getenv("KUBEVIRT_KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = "/etc/rancher/k3s/k3s.yaml"
	}
	if _, err := os.Stat(kubeconfig); err != nil {
		logger.Warn("vms: kubeconfig not present; /vms endpoints will return 503",
			"path", kubeconfig, "err", err)
		return nil
	}

	// TODO(vms): construct the production KubeClient (dynamic client +
	// kubevirt.io/api types) and assign it here. The Manager type, types,
	// templates, console-session minting, and HTTP layer all work today;
	// the client just needs to be plumbed in.
	// Until the dynamic client is plumbed, return nil so the handler
	// 503s every route consistently. Returning a half-wired Manager
	// would risk nil-pointer panics on the first list/get call.
	logger.Warn("vms: KubeClient not yet wired; /vms endpoints will return 503 until the dynamic client lands")
	_ = cat
	return nil
}
