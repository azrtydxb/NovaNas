// Package main provides the NovaNas metadata service binary.
//
// NovaNas is single-node by design (docs/14 S12): the metadata service
// maintains cluster metadata in a local BadgerDB store with durability
// provided by Badger's WAL + fsync. Raft consensus was removed because
// it added operational complexity without value on a single-node
// deployment. The gRPC service interface is unchanged so upstream
// clients (operators, CSI, s3gw) continue to work as before.
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
	dataDir := flag.String("data-dir", "/var/lib/novanas/meta", "Metadata data directory (BadgerDB files are stored under <data-dir>/badger)")
	grpcAddr := flag.String("grpc-addr", ":7001", "gRPC client API listen address")
	metricsAddr := flag.String("metrics-addr", ":7002", "Prometheus metrics listen address")
	tlsCA := flag.String("tls-ca", "", "Path to CA certificate for mTLS")
	tlsCert := flag.String("tls-cert", "", "Path to server certificate for mTLS")
	tlsKey := flag.String("tls-key", "", "Path to server key for mTLS")
	tlsRotationInterval := flag.Duration("tls-rotation-interval", 5*time.Minute, "Interval for TLS certificate rotation checks")
	gcInterval := flag.Duration("gc-interval", 1*time.Hour, "Interval between metadata garbage collection runs")
	gcNodeTTL := flag.Duration("gc-node-ttl", 24*time.Hour, "Time after which a node with no heartbeat is considered stale for GC")
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

	// Create the metadata store (direct BadgerDB; no Raft — docs/14 S12).
	store, err := metadata.NewRaftStore(metadata.RaftConfig{
		NodeID:  *nodeID,
		DataDir: *dataDir,
	})
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
	metaServer := metadata.NewGRPCServer(store)
	metaServer.Register(grpcServer)

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
