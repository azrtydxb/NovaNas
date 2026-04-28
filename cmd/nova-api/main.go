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
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api"
	"github.com/novanas/nova-nas/internal/config"
	"github.com/novanas/nova-nas/internal/host/disks"
	"github.com/novanas/nova-nas/internal/host/iscsi"
	"github.com/novanas/nova-nas/internal/host/krb5"
	"github.com/novanas/nova-nas/internal/host/nfs"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
	"github.com/novanas/nova-nas/internal/host/rdma"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/logging"
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
	nfsMgr := &nfs.Manager{}
	krb5Mgr := &krb5.Manager{}
	rdmaLister := &rdma.Lister{}

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

	dispatcher := &jobs.Dispatcher{
		Client:  asyncClient,
		Queries: st.Queries,
		Pool:    st.Pool,
	}

	// Crash recovery: mark any running/queued rows as interrupted before
	// the worker starts consuming new tasks.
	if err := jobs.MarkInterruptedAtStartup(ctx, st.Queries); err != nil {
		logger.Error("recovery", "err", err)
	}

	asyncSrv := asynq.NewServer(asynqRedisOpt, asynq.Config{
		Concurrency: 4,
		Logger:      asynqSlogAdapter{l: logger},
	})
	mux := jobs.NewServeMux(jobs.WorkerDeps{
		Logger:    logger,
		Queries:   st.Queries,
		Redis:     redisClient,
		Pools:     poolMgr,
		Datasets:  datasetMgr,
		Snapshots: snapMgr,
		IscsiMgr:  iscsiMgr,
		NvmeofMgr: nvmeofMgr,
		NfsMgr:    nfsMgr,
		Krb5Mgr:   krb5Mgr,
	})
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
		RdmaLister:  rdmaLister,
	})
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

type asynqSlogAdapter struct{ l *slog.Logger }

func (a asynqSlogAdapter) Debug(args ...any) { a.l.Debug("asynq", "args", args) }
func (a asynqSlogAdapter) Info(args ...any)  { a.l.Info("asynq", "args", args) }
func (a asynqSlogAdapter) Warn(args ...any)  { a.l.Warn("asynq", "args", args) }
func (a asynqSlogAdapter) Error(args ...any) { a.l.Error("asynq", "args", args) }
func (a asynqSlogAdapter) Fatal(args ...any) { a.l.Error("asynq", "args", args) }
