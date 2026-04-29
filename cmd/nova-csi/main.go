// Command nova-csi is the NovaNAS CSI driver. It listens on a Unix domain
// socket and serves the CSI Identity, Controller, and Node services.
//
// For NovaNAS v1, controller and node responsibilities run in a single binary
// targeting a single-node k3s deployment where the storage host and the
// kubelet share the same machine. In larger deployments the same binary can
// be run as a controller-only Deployment and a node-only DaemonSet by
// disabling capabilities at the proto level (left as future work).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	novanas "github.com/novanas/nova-nas/clients/go/novanas"
	"github.com/novanas/nova-nas/internal/csi"
)

func main() {
	endpoint := flag.String("endpoint", "unix:///csi/csi.sock", "CSI gRPC endpoint")
	nodeID := flag.String("node-id", "", "Node ID (hostname); required")
	novanasURL := flag.String("novanas-url", "", "NovaNAS API base URL")
	novanasToken := flag.String("novanas-token", "", "Bearer token (or use --novanas-token-file)")
	tokenFile := flag.String("novanas-token-file", "/var/run/novanas/token", "File path containing the bearer token (re-read on SIGHUP)")
	caCert := flag.String("novanas-ca-cert", "/etc/nova-ca/ca.crt", "CA cert file for the NovaNAS API")
	defaultPool := flag.String("default-pool", "", "Fallback ZFS pool when the StorageClass omits 'pool'")
	defaultParent := flag.String("default-parent", "", "Default parent dataset (defaults to '<pool>/csi')")
	hostRootPrefix := flag.String("host-root-prefix", "", "When running in a container, prefix prepended to host-namespace paths (e.g. /host) so they resolve under a HostToContainer-propagated bind-mount of host root")
	driverName := flag.String("driver-name", csi.DefaultName, "CSI driver name")
	driverVersion := flag.String("driver-version", csi.DefaultVersion, "CSI driver version")
	logLevel := flag.String("log-level", "info", "log level: debug|info|warn|error")
	flag.Parse()

	logger := newLogger(*logLevel)
	if *nodeID == "" {
		if hn, err := os.Hostname(); err == nil {
			*nodeID = hn
			logger.Info("node-id defaulted to hostname", "node-id", *nodeID)
		} else {
			logger.Error("--node-id is required and could not default to hostname", "err", err)
			os.Exit(2)
		}
	}

	// Build the NovaNAS HTTP client. The real adapter lives in
	// clients/go/novanas; until it lands, main fails closed.
	client, err := buildClient(*novanasURL, *novanasToken, *tokenFile, *caCert, logger)
	if err != nil {
		logger.Error("build NovaNAS client", "err", err)
		os.Exit(1)
	}

	driver := csi.NewDriver(csi.Config{
		Name:          *driverName,
		Version:       *driverVersion,
		NodeID:        *nodeID,
		DefaultPool:   *defaultPool,
		DefaultParent:  *defaultParent,
		HostRootPrefix: *hostRootPrefix,
		Logger:        logger,
	}, client, csi.NewShellMounter())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := driver.Run(ctx, *endpoint); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("nova-csi exited", "err", err)
		os.Exit(1)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

// buildClient builds the NovaNAS HTTP client and adapts it to the
// csi.NovaNASClient interface.
func buildClient(url, token, tokenFile, caCert string, logger *slog.Logger) (csi.NovaNASClient, error) {
	if url == "" {
		return nil, errors.New("--novanas-url is required")
	}
	tok := strings.TrimSpace(token)
	if tok == "" && tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err == nil {
			tok = strings.TrimSpace(string(b))
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read token file %s: %w", tokenFile, err)
		}
	}
	if tok == "" {
		return nil, errors.New("no token provided (--novanas-token or --novanas-token-file)")
	}
	cfg := novanas.Config{BaseURL: url, Token: tok, Timeout: 30 * time.Second}
	if caCert != "" {
		ca, err := os.ReadFile(caCert)
		if err != nil {
			return nil, fmt.Errorf("read CA cert %s: %w", caCert, err)
		}
		cfg.CACertPEM = ca
	}
	c, err := novanas.New(cfg)
	if err != nil {
		return nil, err
	}
	logger.Info("novanas client ready", "url", url, "ca", caCert != "")
	return &sdkAdapter{c: c}, nil
}

// sdkAdapter wraps *novanas.Client to satisfy csi.NovaNASClient. The shapes
// in the SDK and the CSI interface mirror each other but are declared
// independently, so a thin translation layer lives here.
type sdkAdapter struct{ c *novanas.Client }

func (a *sdkAdapter) GetDataset(ctx context.Context, fullname string) (*csi.Dataset, error) {
	d, err := a.c.GetDataset(ctx, fullname)
	if err != nil {
		return nil, err
	}
	return &csi.Dataset{
		Name:           d.Dataset.Name,
		Type:           d.Dataset.Type,
		UsedBytes:      d.Dataset.UsedBytes,
		AvailableBytes: d.Dataset.AvailableBytes,
		Mountpoint:     d.Dataset.Mountpoint,
	}, nil
}

func (a *sdkAdapter) CreateDataset(ctx context.Context, spec csi.CreateDatasetSpec) (*csi.Job, error) {
	j, err := a.c.CreateDataset(ctx, novanas.CreateDatasetSpec{
		Parent: spec.Parent, Name: spec.Name, Type: spec.Type,
		VolumeSizeBytes: spec.VolumeSizeBytes, Properties: spec.Properties,
	})
	return jobOrErr(j, err)
}

func (a *sdkAdapter) DestroyDataset(ctx context.Context, fullname string, recursive bool) (*csi.Job, error) {
	j, err := a.c.DestroyDataset(ctx, fullname, recursive)
	return jobOrErr(j, err)
}

func (a *sdkAdapter) SetDatasetProps(ctx context.Context, fullname string, props map[string]string) (*csi.Job, error) {
	j, err := a.c.SetDatasetProps(ctx, fullname, props)
	return jobOrErr(j, err)
}

func (a *sdkAdapter) CreateSnapshot(ctx context.Context, dataset, shortName string, recursive bool) (*csi.Job, error) {
	j, err := a.c.CreateSnapshot(ctx, dataset, shortName, recursive)
	return jobOrErr(j, err)
}

func (a *sdkAdapter) DestroySnapshot(ctx context.Context, fullname string) (*csi.Job, error) {
	j, err := a.c.DestroySnapshot(ctx, fullname)
	return jobOrErr(j, err)
}

func (a *sdkAdapter) CloneSnapshot(ctx context.Context, snapshot, target string, properties map[string]string) (*csi.Job, error) {
	j, err := a.c.CloneSnapshot(ctx, snapshot, target, properties)
	return jobOrErr(j, err)
}

func (a *sdkAdapter) WaitJob(ctx context.Context, id string, pollInterval time.Duration) (*csi.Job, error) {
	j, err := a.c.WaitJob(ctx, id, pollInterval)
	return jobOrErr(j, err)
}

func (a *sdkAdapter) IsNotFound(err error) bool { return novanas.IsNotFound(err) }

func jobOrErr(j *novanas.Job, err error) (*csi.Job, error) {
	if err != nil {
		return nil, err
	}
	return &csi.Job{ID: j.ID, State: j.State, Error: j.Error}, nil
}
