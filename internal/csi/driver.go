package csi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	csipb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
)

// Defaults.
const (
	DefaultName    = "csi.novanas.io"
	DefaultVersion = "0.1.0"

	// JobPollInterval is how often we poll long-running NovaNAS jobs.
	JobPollInterval = 1 * time.Second
)

// Config is the driver configuration assembled by main.
type Config struct {
	Name          string
	Version       string
	NodeID        string
	DefaultPool   string
	DefaultParent string // if empty, derived as "<pool>/csi"
	Logger        *slog.Logger

	// HostRootPrefix is prepended to dataset mountpoints and zvol device
	// paths during Node mount operations. Empty means "no prefix" (the
	// driver runs on the host directly). When the driver runs in a
	// container, set this to where the host root is bind-mounted with
	// HostToContainer propagation, e.g. "/host". Then a dataset whose
	// mountpoint is "/tank/csi/foo" is bind-mounted from
	// "/host/tank/csi/foo" — propagation makes the host's auto-mounted
	// child datasets visible.
	HostRootPrefix string

	// NFSServer is the FQDN or IP that pods (and this DaemonSet's
	// mount.nfs4 invocation) use to reach the NovaNAS NFS server. It is
	// embedded in the VolumeContext for kind=nfs volumes so the Node
	// service can mount without re-resolving.
	NFSServer string

	// DefaultNFSClients is the comma-separated client allowlist applied to
	// new NFS exports when the StorageClass does not override
	// "nfsClients". Each entry follows the NFS export rule format (CIDR,
	// IP, "*", or hostname/wildcard).
	DefaultNFSClients string
}

// Driver is the shared state between the three CSI services.
type Driver struct {
	cfg     Config
	client  NovaNASClient
	mounter Mounter
	log     *slog.Logger
}

// NewDriver constructs a Driver. Name/Version default if blank.
func NewDriver(cfg Config, client NovaNASClient, mounter Mounter) *Driver {
	if cfg.Name == "" {
		cfg.Name = DefaultName
	}
	if cfg.Version == "" {
		cfg.Version = DefaultVersion
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Driver{cfg: cfg, client: client, mounter: mounter, log: cfg.Logger}
}

// hostPath rewrites a host-namespace path so it's reachable from inside
// the CSI Node container. Returns p unchanged when HostRootPrefix is
// empty (driver running on the host directly).
func (d *Driver) hostPath(p string) string {
	if d.cfg.HostRootPrefix == "" || p == "" {
		return p
	}
	if strings.HasPrefix(p, d.cfg.HostRootPrefix) {
		return p
	}
	return d.cfg.HostRootPrefix + p
}

// defaultParent returns the resolved default parent dataset path.
func (d *Driver) defaultParent(pool string) string {
	if d.cfg.DefaultParent != "" {
		return d.cfg.DefaultParent
	}
	if pool == "" {
		pool = d.cfg.DefaultPool
	}
	if pool == "" {
		return ""
	}
	return pool + "/csi"
}

// Run starts the gRPC server on endpoint until ctx is cancelled. Endpoint
// supports both "unix://..." and "tcp://..." for tests.
func (d *Driver) Run(ctx context.Context, endpoint string) error {
	network, addr, err := parseEndpoint(endpoint)
	if err != nil {
		return err
	}
	if network == "unix" {
		_ = os.Remove(addr)
		if err := os.MkdirAll(parentDir(addr), 0o755); err != nil {
			return fmt.Errorf("mkdir socket dir: %w", err)
		}
	}
	lis, err := net.Listen(network, addr)
	if err != nil {
		return fmt.Errorf("listen %s://%s: %w", network, addr, err)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(d.logInterceptor))
	csipb.RegisterIdentityServer(srv, &IdentityService{d: d})
	csipb.RegisterControllerServer(srv, &ControllerService{d: d})
	csipb.RegisterNodeServer(srv, &NodeService{d: d})

	d.log.Info("nova-csi listening", "endpoint", endpoint, "name", d.cfg.Name, "version", d.cfg.Version)
	go func() {
		<-ctx.Done()
		srv.GracefulStop()
	}()
	if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}

func (d *Driver) logInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	d.log.Debug("rpc", "method", info.FullMethod, "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	return resp, err
}

func parseEndpoint(ep string) (string, string, error) {
	u, err := url.Parse(ep)
	if err != nil {
		return "", "", fmt.Errorf("parse endpoint: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "unix":
		// unix:///path → host empty, path is /path
		p := u.Path
		if p == "" {
			p = u.Opaque
		}
		return "unix", p, nil
	case "tcp":
		return "tcp", u.Host, nil
	default:
		return "", "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
}
