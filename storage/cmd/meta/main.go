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

	"go.uber.org/zap"

	"github.com/azrtydxb/novanas/storage/internal/dataplane"
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
	chunkBacked := flag.Bool("chunk-backed", false, "Enable chunk-backed metadata (A4-Metadata-As-Chunks). When true, bootstrap via superblocks and mount the metadata BlockVolume via the data-plane's ExportMetadataVolumeNBD RPC before opening BadgerDB. If that RPC returns Unimplemented the meta service falls back to --data-dir unless --require-chunk-backed is set.")
	metaMountPath := flag.String("meta-mount-path", "/var/lib/novanas/meta-mount", "Mount path for the chunk-backed metadata BlockVolume when --chunk-backed=true.")
	bootstrapTimeout := flag.Duration("bootstrap-timeout", 30*time.Second, "How long to wait for agents to report metadata-role superblocks before failing startup (chunk-backed mode only).")
	minMetaDisks := flag.Int("min-metadata-disks", 1, "Minimum number of metadata-role disks that must report a superblock before bootstrap proceeds.")
	dataplaneAddr := flag.String("dataplane-addr", "127.0.0.1:9500", "gRPC address of the local data-plane used for ExportMetadataVolumeNBD in chunk-backed mode.")
	requireChunkBacked := flag.Bool("require-chunk-backed", false, "In chunk-backed mode, fail startup if the data-plane cannot mount the metadata BlockVolume. When false (default), fall back to --data-dir.")
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
	// Chunk-backed path (A4-Metadata-As-Chunks, docs/14 S14): bootstrap
	// by gathering metadata-role superblocks from agents, verifying
	// CRUSH agreement, calling DataplaneService.ExportMetadataVolumeNBD
	// to assemble the metadata BlockVolume as /dev/nbdN, mounting it at
	// --meta-mount-path, and opening BadgerDB there. If the dataplane
	// has not yet implemented the export RPC (returns Unimplemented),
	// NewRaftStoreChunkBackedMounted surfaces ErrMountNotSupported and
	// we fall back to --data-dir.
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
		bootCtx, bootCancel := context.WithTimeout(context.Background(), *bootstrapTimeout)
		// Production mounter: delegate to the data-plane's
		// ExportMetadataVolumeNBD / ReleaseMetadataVolumeNBD RPCs via
		// the gRPC client at --dataplane-addr. If the data-plane does
		// not yet implement the RPC (returns Unimplemented),
		// NewRaftStoreChunkBackedMounted detects ErrMountNotSupported
		// and falls back to --data-dir unless --require-chunk-backed
		// is set.
		var mounter metadata.MetadataVolumeMounter = metadata.NoopMetadataVolumeMounter{}
		if dpClient, dpErr := dataplane.Dial(*dataplaneAddr, zap.NewNop()); dpErr == nil {
			defer func() { _ = dpClient.Close() }()
			mounter = &metadata.DataplaneNBDMounter{
				Export: func(ctx context.Context, loc metadata.VolumeLocator) (string, error) {
					return dpClient.ExportMetadataVolumeNBD(ctx, loc.Name, loc.RootChunk, loc.Version)
				},
				Release: func(ctx context.Context, loc metadata.VolumeLocator) (string, error) {
					return "", dpClient.ReleaseMetadataVolumeNBD(ctx, loc.Name, "")
				},
			}
		} else {
			log.Printf("dataplane dial at %s failed (%v); chunk-backed mount path disabled", *dataplaneAddr, dpErr)
		}
		var report *metadata.BootstrapReport
		store, report, err = metadata.NewRaftStoreChunkBackedMounted(bootCtx, *nodeID, metadata.BootstrapConfig{
			LocalDataDir:       *dataDir,
			ChunkBackedEnabled: true,
			MetaMountPath:      *metaMountPath,
			BootstrapTimeout:   *bootstrapTimeout,
			MinMetadataDisks:   *minMetaDisks,
		}, sbRegistry, mounter)
		bootCancel()
		if report != nil {
			log.Printf("chunk-backed bootstrap report: disks=%d meta-volume=%s root=%s ver=%d",
				report.MetadataDisks, report.MetadataVolumeName, report.MetadataVolumeRoot, report.MetadataVolumeVer)
		}
		if err != nil {
			if *requireChunkBacked {
				log.Fatalf("chunk-backed bootstrap failed and --require-chunk-backed=true: %v", err)
			}
			log.Printf("chunk-backed bootstrap did not succeed (%v); using --data-dir fallback", err)
			store, err = metadata.NewRaftStore(metadata.RaftConfig{NodeID: *nodeID, DataDir: *dataDir})
		}
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

// Previously this file contained a noopSuperblockSource placeholder.
// Replaced by metadata.NewSuperblockRegistry() from A10-API-Infra wiring.
