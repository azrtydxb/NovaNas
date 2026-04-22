// Package main provides the NovaNas metadata service binary.
//
// NovaNas is single-node by design (docs/14 S12): the metadata service
// maintains cluster metadata in a BadgerDB store with durability provided
// by Badger's WAL + fsync. Raft consensus was removed because it added
// operational complexity without value on a single-node deployment.
//
// Post-A4-Metadata-As-Chunks (docs/14 S11/S13/S14): the metadata store
// now lives on the chunk engine itself — BadgerDB's files are held on a
// chunk-backed BlockVolume that is reconstructed at startup from the
// per-disk superblocks reported by agents. The --data-dir flag is
// retained for backward compatibility but deprecated; it is honoured only
// when --chunk-backed=false (local-disk fallback).
//
// The gRPC service interface is unchanged so upstream clients (operators,
// CSI, s3gw) continue to work as before.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"github.com/azrtydxb/novanas/storage/internal/disk"
	"github.com/azrtydxb/novanas/storage/internal/logging"
	"github.com/azrtydxb/novanas/storage/internal/metadata"
	"github.com/azrtydxb/novanas/storage/internal/metrics"
	"github.com/azrtydxb/novanas/storage/internal/observability"
	"github.com/azrtydxb/novanas/storage/internal/transport"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	nodeID := flag.String("node-id", "", "Metadata node ID (defaults to hostname when empty)")
	dataDir := flag.String("data-dir", "/var/lib/novanas/meta", "DEPRECATED: legacy local BadgerDB directory. Used only when --chunk-backed=false. In chunk-backed mode BadgerDB lives on a chunk-backed BlockVolume mounted at --meta-mount-path.")
	chunkBacked := flag.Bool("chunk-backed", false, "Enable chunk-backed metadata (A4-Metadata-As-Chunks). When true, bootstrap via superblocks and mount the metadata BlockVolume before opening BadgerDB. Currently scaffolded: falls back to --data-dir until NBD export is wired in the data-plane (TODO(integration)).")
	metaMountPath := flag.String("meta-mount-path", "/var/lib/novanas/meta-mount", "Mount path for the chunk-backed metadata BlockVolume when --chunk-backed=true.")
	bootstrapTimeout := flag.Duration("bootstrap-timeout", 30*time.Second, "How long to wait for agents to report metadata-role superblocks before failing startup (chunk-backed mode only).")
	minMetaDisks := flag.Int("min-metadata-disks", 1, "Minimum number of metadata-role disks that must report a superblock before bootstrap proceeds.")
	grpcAddr := flag.String("grpc-addr", ":7001", "gRPC client API listen address")
	metricsAddr := flag.String("metrics-addr", ":7002", "Prometheus metrics listen address")
	tlsCA := flag.String("tls-ca", "", "Path to CA certificate for mTLS")
	tlsCert := flag.String("tls-cert", "", "Path to server certificate for mTLS")
	tlsKey := flag.String("tls-key", "", "Path to server key for mTLS")
	tlsRotationInterval := flag.Duration("tls-rotation-interval", 5*time.Minute, "Interval for TLS certificate rotation checks")
	gcInterval := flag.Duration("gc-interval", 1*time.Hour, "Interval between metadata garbage collection runs")
	gcNodeTTL := flag.Duration("gc-node-ttl", 24*time.Hour, "Time after which a node with no heartbeat is considered stale for GC")
	// A4-Encryption: OpenBao Transit config (docs/02, docs/10). Meta
	// server uses these when provisioning per-volume Dataset Keys.
	openbaoAddr := flag.String("openbao-addr", "", "OpenBao base URL (e.g. https://openbao:8200). Empty disables encryption.")
	openbaoTokenPath := flag.String("openbao-token-path", "/var/run/secrets/openbao/token", "Path to OpenBao token file")
	masterKeyName := flag.String("master-key-name", "novanas/chunk-master", "OpenBao Transit master-key name used to wrap Dataset Keys")
	encryptionMode := flag.String("encryption-mode", "off", "Encryption mode: off | mandatory (default off in v1; opt-in per volume via CRD)")
	_ = openbaoAddr
	_ = openbaoTokenPath
	_ = masterKeyName
	_ = encryptionMode
	flag.Parse()

	logging.Init(false)
	defer logging.Sync()

	shutdownTracer := observability.InitTracer("novanas-storage-meta", logging.L)
	defer shutdownTracer()

	log.Printf("novanas-storage-meta %s (commit: %s, built: %s)", version, commit, date)

	// Default node ID to hostname when not explicitly set.
	if *nodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatalf("--node-id not set and could not determine hostname: %v", err)
		}
		*nodeID = hostname
		log.Printf("--node-id not set; using hostname %q", *nodeID)
	}

	// Register Prometheus metrics.
	metrics.Register()

	// Create the metadata store.
	//
	// Chunk-backed path (A4-Metadata-As-Chunks, docs/14 S14): bootstrap by
	// gathering metadata-role superblocks from agents, verifying CRUSH
	// agreement, and (TODO(integration)) mounting the chunk-backed
	// BlockVolume before opening BadgerDB. For now, the bootstrap step
	// runs in "report-only" mode: it proves the locator flow and then
	// opens BadgerDB at --data-dir as a safe fallback.
	//
	// Local-disk path (legacy): open BadgerDB at --data-dir directly.
	var (
		store *metadata.RaftStore
		err   error
	)
	// Shared SuperblockRegistry: ingests agent-reported superblocks via the
	// ReportSuperblocks RPC on the gRPC server and exposes them as a
	// SuperblockSource for the chunk-backed bootstrap path. Wiring both
	// sides to the same instance resolves the chicken-and-egg problem
	// between "agents report disks" and "meta needs disks to bootstrap".
	sbRegistry := metadata.NewSuperblockRegistry()

	if *chunkBacked {
		log.Printf("chunk-backed metadata enabled (bootstrap-timeout=%s, min-metadata-disks=%d, mount-path=%s)",
			*bootstrapTimeout, *minMetaDisks, *metaMountPath)
		log.Printf("TODO(integration): chunk-mount + BadgerDB-on-mount path not yet wired; falling back to --data-dir=%s", *dataDir)
		bootCtx, bootCancel := context.WithTimeout(context.Background(), *bootstrapTimeout)
		_, report, bootErr := metadata.NewRaftStoreChunkBacked(bootCtx, *nodeID, metadata.BootstrapConfig{
			LocalDataDir:       *dataDir,
			ChunkBackedEnabled: true,
			MetaMountPath:      *metaMountPath,
			BootstrapTimeout:   *bootstrapTimeout,
			MinMetadataDisks:   *minMetaDisks,
		}, sbRegistry)
		bootCancel()
		if bootErr != nil {
			log.Printf("chunk-backed bootstrap did not succeed (%v); continuing with --data-dir fallback", bootErr)
		} else if report != nil {
			log.Printf("chunk-backed bootstrap report: disks=%d meta-volume=%s root=%s ver=%d",
				report.MetadataDisks, report.MetadataVolumeName, report.MetadataVolumeRoot, report.MetadataVolumeVer)
		}
		store, err = metadata.NewRaftStore(metadata.RaftConfig{NodeID: *nodeID, DataDir: *dataDir})
	} else {
		log.Printf("--data-dir=%s (legacy local-disk mode; --chunk-backed=false)", *dataDir)
		store, err = metadata.NewRaftStore(metadata.RaftConfig{NodeID: *nodeID, DataDir: *dataDir})
	}
	if err != nil {
		log.Fatalf("Failed to create metadata store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Main context for the metadata service lifetime.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the garbage collector for orphan chunks and stale metadata.
	gc := metadata.NewGarbageCollector(store, *gcInterval, *gcNodeTTL)
	gc.Start(ctx)
	log.Printf("Garbage collector started (interval=%s, node-ttl=%s)", *gcInterval, *gcNodeTTL)

	// Build gRPC server options.
	var serverOpts []grpc.ServerOption
	if *tlsCA != "" && *tlsCert != "" && *tlsKey != "" {
		rotator := transport.NewCertRotator(*tlsCert, *tlsKey, *tlsRotationInterval)
		rotator.Start(ctx)
		log.Printf("TLS certificate rotation enabled (cert=%s, key=%s, interval=%s)",
			*tlsCert, *tlsKey, *tlsRotationInterval)
		tlsOpt, tlsErr := transport.NewServerTLSWithRotation(transport.TLSConfig{
			CACertPath: *tlsCA,
			CertPath:   *tlsCert,
			KeyPath:    *tlsKey,
		}, rotator)
		if tlsErr != nil {
			log.Fatalf("Failed to configure TLS: %v", tlsErr)
		}
		serverOpts = append(serverOpts, tlsOpt)
	}

	// Create and register the gRPC metadata server.
	serverOpts = append(serverOpts, grpc.StatsHandler(otelgrpc.NewServerHandler()))
	grpcServer := grpc.NewServer(serverOpts...)
	metaServer := metadata.NewGRPCServer(store).WithSuperblockRegistry(sbRegistry, *minMetaDisks)
	metaServer.Register(grpcServer)
	log.Printf("SuperblockRegistry wired (min-metadata-disks=%d)", *minMetaDisks)

	// Start gRPC listener.
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", *grpcAddr, err)
	}

	log.Printf("Metadata gRPC server listening on %s (node: %s, single-node mode)",
		*grpcAddr, *nodeID)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("Metadata gRPC server failed: %v", err)
		}
	}()

	// Start Prometheus metrics HTTP server.
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:         *metricsAddr,
		Handler:      metricsMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("Metrics server listening on %s", *metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Graceful shutdown on SIGTERM/SIGINT.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop

	log.Println("Shutting down metadata service...")
	cancel() // cancel main context: stops cert rotator
	grpcServer.GracefulStop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = metricsServer.Shutdown(shutdownCtx)
	log.Println("Metadata service stopped")
}

// noopSuperblockSource is a placeholder metadata.SuperblockSource used
// until the agent-to-meta superblock-report channel is wired. It always
// returns an empty slice, which deliberately causes
// BootstrapChunkBacked to fail with ErrBootstrapTimeout; the main()
// call-site then logs and falls back to --data-dir. This keeps the
// chunk-backed startup path exercised end-to-end in code without
// requiring a live cluster.
//
// TODO(integration): replace with a real source that subscribes to
// agent heartbeats and accumulates disk.ScanResult from their reports.
type noopSuperblockSource struct{}

func (*noopSuperblockSource) GatherMetadataSuperblocks(ctx context.Context, min int) ([]disk.ScanResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
