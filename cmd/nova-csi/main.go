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
	oidcIssuer := flag.String("oidc-issuer-url", "", "Keycloak realm issuer URL (e.g. https://kc:8443/realms/novanas). When set, enables automatic token refresh via client_credentials.")
	oidcClientID := flag.String("oidc-client-id", "", "OIDC client ID for client_credentials grant (mutually exclusive with --oidc-client-id-file)")
	oidcClientIDFile := flag.String("oidc-client-id-file", "", "Path to file containing OIDC client ID (alternative to --oidc-client-id)")
	oidcClientSecretFile := flag.String("oidc-client-secret-file", "", "Path to file containing OIDC client secret")
	oidcCACert := flag.String("oidc-ca-cert", "", "CA cert file for the OIDC issuer's TLS (defaults to --novanas-ca-cert)")
	defaultPool := flag.String("default-pool", "", "Fallback ZFS pool when the StorageClass omits 'pool'")
	defaultParent := flag.String("default-parent", "", "Default parent dataset (defaults to '<pool>/csi')")
	hostRootPrefix := flag.String("host-root-prefix", "", "When running in a container, prefix prepended to host-namespace paths (e.g. /host) so they resolve under a HostToContainer-propagated bind-mount of host root")
	nfsServer := flag.String("nfs-server", "", "FQDN or IP that pods reach to mount NFS exports created by kind=nfs StorageClasses")
	defaultNFSClients := flag.String("default-nfs-clients", "", "Comma-separated CIDR/IP/wildcard allowlist applied to new NFS exports when the StorageClass omits 'nfsClients'")
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Build the NovaNAS HTTP client. The real adapter lives in
	// clients/go/novanas; until it lands, main fails closed.
	client, sdkClient, err := buildClient(*novanasURL, *novanasToken, *tokenFile, *caCert, logger)
	if err != nil {
		logger.Error("build NovaNAS client", "err", err)
		os.Exit(1)
	}

	// If OIDC flags are supplied, swap the static token for a refreshing
	// client_credentials source. We perform the initial fetch synchronously
	// and fail closed if it errors — see fetchInitialToken for rationale.
	oidcEnabled := *oidcIssuer != "" || *oidcClientID != "" || *oidcClientIDFile != "" || *oidcClientSecretFile != ""
	if oidcEnabled {
		if err := setupOIDCRefresh(ctx, sdkClient, *oidcIssuer, *oidcClientID, *oidcClientIDFile, *oidcClientSecretFile, *oidcCACert, *caCert, logger); err != nil {
			logger.Error("oidc setup failed", "err", err)
			os.Exit(1)
		}
	} else if sdkClient.Token == "" {
		// Legacy static-token path: bearer is mandatory.
		logger.Error("no token provided (--novanas-token, --novanas-token-file, or --oidc-* flags)")
		os.Exit(1)
	}

	driver, err := csi.NewDriver(csi.Config{
		Name:              *driverName,
		Version:           *driverVersion,
		NodeID:            *nodeID,
		DefaultPool:       *defaultPool,
		DefaultParent:     *defaultParent,
		HostRootPrefix:    *hostRootPrefix,
		NFSServer:         *nfsServer,
		DefaultNFSClients: *defaultNFSClients,
		Logger:            logger,
	}, client, csi.NewShellMounter())
	if err != nil {
		logger.Error("driver init", "err", err)
		os.Exit(1)
	}

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
// csi.NovaNASClient interface. It also returns the underlying *novanas.Client
// so callers can install a token-refresh loop via SetToken.
//
// If OIDC is in use the caller will overwrite the bearer immediately after
// startup; in that case the static-token discovery here is best-effort and
// an empty token is permitted. Otherwise we keep the legacy "static token
// required" semantics.
func buildClient(url, token, tokenFile, caCert string, logger *slog.Logger) (csi.NovaNASClient, *novanas.Client, error) {
	if url == "" {
		return nil, nil, errors.New("--novanas-url is required")
	}
	tok := strings.TrimSpace(token)
	if tok == "" && tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err == nil {
			tok = strings.TrimSpace(string(b))
		} else if !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("read token file %s: %w", tokenFile, err)
		}
	}
	cfg := novanas.Config{BaseURL: url, Token: tok, Timeout: 30 * time.Second}
	if caCert != "" {
		ca, err := os.ReadFile(caCert)
		if err != nil {
			return nil, nil, fmt.Errorf("read CA cert %s: %w", caCert, err)
		}
		cfg.CACertPEM = ca
	}
	c, err := novanas.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("novanas client ready", "url", url, "ca", caCert != "", "static_token", tok != "")
	return &sdkAdapter{c: c}, c, nil
}

// setupOIDCRefresh validates OIDC flags, performs the initial token fetch,
// installs the resulting bearer on the SDK client, and starts the
// background refresh goroutine. Fails closed on the initial fetch.
func setupOIDCRefresh(ctx context.Context, sdk *novanas.Client, issuer, clientID, clientIDFile, clientSecretFile, oidcCAPath, fallbackCAPath string, logger *slog.Logger) error {
	if issuer == "" {
		return errors.New("--oidc-issuer-url is required when any OIDC flag is set")
	}
	id := strings.TrimSpace(clientID)
	if id == "" && clientIDFile != "" {
		b, err := os.ReadFile(clientIDFile)
		if err != nil {
			return fmt.Errorf("read oidc client id file %s: %w", clientIDFile, err)
		}
		id = strings.TrimSpace(string(b))
	}
	if id == "" {
		return errors.New("--oidc-client-id or --oidc-client-id-file is required")
	}
	if clientSecretFile == "" {
		return errors.New("--oidc-client-secret-file is required")
	}
	secB, err := os.ReadFile(clientSecretFile)
	if err != nil {
		return fmt.Errorf("read oidc client secret %s: %w", clientSecretFile, err)
	}
	secret := strings.TrimSpace(string(secB))
	if secret == "" {
		return fmt.Errorf("oidc client secret file %s is empty", clientSecretFile)
	}

	caPath := oidcCAPath
	if caPath == "" {
		caPath = fallbackCAPath
	}
	var caPEM []byte
	if caPath != "" {
		b, err := os.ReadFile(caPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read oidc CA cert %s: %w", caPath, err)
		}
		caPEM = b
	}

	src, err := newOIDCSource(issuer, id, secret, caPEM)
	if err != nil {
		return err
	}

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tok, exp, err := fetchInitialToken(initCtx, src, logger)
	if err != nil {
		return fmt.Errorf("initial oidc token fetch: %w", err)
	}
	sdk.SetToken(tok)

	go runOIDCRefresh(ctx, src, sdk, exp, logger)
	logger.Info("oidc refresh loop started", "issuer", issuer, "client_id", id)
	return nil
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

func (a *sdkAdapter) CreateProtocolShare(ctx context.Context, share csi.ProtocolShareSpec) (*csi.Job, error) {
	protos := make([]novanas.Protocol, 0, len(share.Protocols))
	for _, p := range share.Protocols {
		protos = append(protos, novanas.Protocol(p))
	}
	clients := make([]novanas.NfsClientRule, 0, len(share.NFSClients))
	for _, c := range share.NFSClients {
		clients = append(clients, novanas.NfsClientRule{Spec: c.Spec, Options: c.Options})
	}
	body := novanas.ProtocolShare{
		Name:        share.Name,
		Pool:        share.Pool,
		DatasetName: share.DatasetName,
		Protocols:   protos,
		Acls:        []novanas.DatasetACE{},
	}
	if share.QuotaBytes > 0 {
		q := share.QuotaBytes
		body.QuotaBytes = &q
	}
	if len(clients) > 0 || containsString(share.Protocols, "nfs") {
		body.NFS = &novanas.ProtocolNFSOpts{Clients: clients}
	}
	j, err := a.c.CreateProtocolShare(ctx, body)
	return jobOrErr(j, err)
}

func (a *sdkAdapter) GetProtocolShare(ctx context.Context, name, pool, dataset string) (*csi.ProtocolShareDetail, error) {
	d, err := a.c.GetProtocolShare(ctx, name, pool, dataset)
	if err != nil {
		return nil, err
	}
	return &csi.ProtocolShareDetail{
		Name:        d.Share.Name,
		Pool:        d.Share.Pool,
		DatasetName: d.Share.DatasetName,
		Path:        d.Path,
	}, nil
}

func (a *sdkAdapter) DeleteProtocolShare(ctx context.Context, name, pool, dataset string) (*csi.Job, error) {
	j, err := a.c.DeleteProtocolShare(ctx, name, pool, dataset)
	return jobOrErr(j, err)
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func (a *sdkAdapter) IsNotFound(err error) bool { return novanas.IsNotFound(err) }

func jobOrErr(j *novanas.Job, err error) (*csi.Job, error) {
	if err != nil {
		return nil, err
	}
	return &csi.Job{ID: j.ID, State: j.State, Error: j.Error}, nil
}
