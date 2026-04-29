// Command nova-krb5-sync reconciles Keycloak users with the embedded MIT
// KDC's principal database, creating Kerberos principals for every user
// with a `nova-tenant` attribute (and platform-level NFS users with
// `nova-platform-nfs: true`). See docs/krb5/sync.md for the operator-
// facing documentation.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	novanas "github.com/novanas/nova-nas/clients/go/novanas"
	"github.com/novanas/nova-nas/internal/krb5sync"

	"gopkg.in/yaml.v3"
)

// fileFlags collects all command-line flags. We use plain flag instead of
// cobra/viper to keep the daemon's surface and dependency footprint tiny.
type fileFlags struct {
	configPath    string
	reconcileOnce bool
	logLevel      string
}

// daemonConfig is the YAML-parsed configuration. All fields are optional
// at parse time; required-ness is enforced after merge with flag/env.
type daemonConfig struct {
	NovaAPI struct {
		BaseURL    string `yaml:"baseURL"`
		CACertPath string `yaml:"caCertPath"`
	} `yaml:"novaAPI"`
	OIDC struct {
		IssuerURL        string `yaml:"issuerURL"`
		ClientID         string `yaml:"clientID"`
		ClientSecretFile string `yaml:"clientSecretFile"`
		CACertPath       string `yaml:"caCertPath"`
	} `yaml:"oidc"`
	Keycloak struct {
		AdminURL           string `yaml:"adminURL"`
		Realm              string `yaml:"realm"`
		CACertPath         string `yaml:"caCertPath"`
		InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
	} `yaml:"keycloak"`
	Krb5 struct {
		Realm string `yaml:"realm"`
	} `yaml:"krb5"`
	Sync struct {
		StateFile     string        `yaml:"stateFile"`
		PollInterval  time.Duration `yaml:"pollInterval"`
		EventInterval time.Duration `yaml:"eventInterval"`
	} `yaml:"sync"`
}

func main() {
	var ff fileFlags
	flag.StringVar(&ff.configPath, "config", "/etc/nova-krb5-sync/config.yaml", "Path to YAML config")
	flag.BoolVar(&ff.reconcileOnce, "reconcile-once", false, "Perform a single reconcile and exit (cron / one-shot mode)")
	flag.StringVar(&ff.logLevel, "log-level", "info", "log level: debug|info|warn|error")
	flag.Parse()

	logger := newLogger(ff.logLevel)

	cfg, err := loadConfig(ff.configPath)
	if err != nil {
		logger.Error("load config", "path", ff.configPath, "err", err)
		os.Exit(1)
	}
	if err := validateConfig(cfg); err != nil {
		logger.Error("invalid config", "err", err)
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rec, cleanup, err := buildReconciler(ctx, cfg, logger)
	if err != nil {
		logger.Error("build reconciler", "err", err)
		os.Exit(1)
	}
	defer cleanup()

	if ff.reconcileOnce {
		res, err := rec.ReconcileOnce(ctx)
		if err != nil {
			logger.Error("reconcile once", "err", err)
			os.Exit(1)
		}
		// Persist state.
		if err := saveState(cfg.Sync.StateFile, rec); err != nil {
			logger.Error("save state", "err", err)
			os.Exit(1)
		}
		logger.Info("reconcile complete",
			"users", res.UsersConsidered,
			"desired", res.PrincipalsDesired,
			"created", len(res.Created),
			"deleted", len(res.Deleted),
			"createErrors", res.CreateErrors,
			"deleteErrors", res.DeleteErrors)
		return
	}

	// Daemon mode: install a state-saver that runs after each reconcile via
	// a periodic save in a goroutine, plus a final save on shutdown.
	stop := make(chan struct{})
	saveDone := make(chan struct{})
	go func() {
		defer close(saveDone)
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				_ = saveState(cfg.Sync.StateFile, rec)
				return
			case <-t.C:
				if err := saveState(cfg.Sync.StateFile, rec); err != nil {
					logger.Warn("periodic state save failed", "err", err)
				}
			}
		}
	}()

	firstReady := func() { sdNotify("READY=1\nSTATUS=initial reconcile complete\n", logger) }
	if err := rec.Run(ctx, firstReady); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("reconciler exited", "err", err)
	}
	close(stop)
	<-saveDone
	logger.Info("nova-krb5-sync exiting")
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
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

func loadConfig(path string) (*daemonConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg daemonConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Defaults.
	if cfg.Sync.StateFile == "" {
		cfg.Sync.StateFile = "/var/lib/nova-krb5-sync/state.json"
	}
	if cfg.Sync.PollInterval == 0 {
		cfg.Sync.PollInterval = 5 * time.Minute
	}
	if cfg.Sync.EventInterval == 0 {
		cfg.Sync.EventInterval = 30 * time.Second
	}
	if cfg.Krb5.Realm == "" {
		cfg.Krb5.Realm = "NOVANAS.LOCAL"
	}
	if cfg.Keycloak.Realm == "" {
		cfg.Keycloak.Realm = "novanas"
	}
	return &cfg, nil
}

func validateConfig(cfg *daemonConfig) error {
	if cfg.NovaAPI.BaseURL == "" {
		return errors.New("novaAPI.baseURL is required")
	}
	if cfg.Keycloak.AdminURL == "" {
		return errors.New("keycloak.adminURL is required")
	}
	if cfg.OIDC.IssuerURL == "" || cfg.OIDC.ClientID == "" || cfg.OIDC.ClientSecretFile == "" {
		return errors.New("oidc.issuerURL, oidc.clientID, oidc.clientSecretFile are all required")
	}
	return nil
}

// buildReconciler wires up the Keycloak client, the novanas SDK client
// (with OIDC client_credentials), and the state file. The returned
// cleanup closes resources.
func buildReconciler(ctx context.Context, cfg *daemonConfig, logger *slog.Logger) (*krb5sync.Reconciler, func(), error) {
	// Read OIDC client secret.
	secB, err := os.ReadFile(cfg.OIDC.ClientSecretFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read oidc client secret %s: %w", cfg.OIDC.ClientSecretFile, err)
	}
	clientSecret := strings.TrimSpace(string(secB))
	if clientSecret == "" {
		return nil, nil, fmt.Errorf("oidc client secret %s is empty", cfg.OIDC.ClientSecretFile)
	}

	novaCAPEM, err := readOptionalFile(cfg.NovaAPI.CACertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read novaAPI CA: %w", err)
	}
	kcCAPEM, err := readOptionalFile(cfg.Keycloak.CACertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read keycloak CA: %w", err)
	}

	// 1. SDK client to nova-api with OIDC bearer.
	sdk, err := novanas.New(novanas.Config{BaseURL: cfg.NovaAPI.BaseURL, CACertPEM: novaCAPEM})
	if err != nil {
		return nil, nil, err
	}
	novaTokSrc := &novaTokenSource{
		tokenURL:     strings.TrimRight(cfg.OIDC.IssuerURL, "/") + "/protocol/openid-connect/token",
		clientID:     cfg.OIDC.ClientID,
		clientSecret: clientSecret,
	}
	if err := novaTokSrc.init(novaCAPEM); err != nil {
		return nil, nil, err
	}
	tok, exp, err := novaTokSrc.fetch(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("initial nova-api oidc token: %w", err)
	}
	sdk.SetToken(tok)
	logger.Info("nova-api oidc token acquired", "exp", exp.UTC().Format(time.RFC3339))
	// Background refresh loop.
	refreshCtx, cancelRefresh := context.WithCancel(ctx)
	go novaTokSrc.runRefresh(refreshCtx, sdk, exp, logger)

	// 2. Keycloak admin client. Reuses the same client_credentials secret;
	// the `nova-krb5-sync` client must have realm-management/view-users
	// and view-events role mappings.
	kc, err := krb5sync.NewKeycloakClient(ctx, krb5sync.KeycloakConfig{
		BaseURL:            cfg.Keycloak.AdminURL,
		Realm:              cfg.Keycloak.Realm,
		ClientID:           cfg.OIDC.ClientID,
		ClientSecret:       clientSecret,
		CACertPEM:          kcCAPEM,
		InsecureSkipVerify: cfg.Keycloak.InsecureSkipVerify,
	})
	if err != nil {
		cancelRefresh()
		return nil, nil, err
	}

	// 3. State.
	if err := os.MkdirAll(filepath.Dir(cfg.Sync.StateFile), 0o755); err != nil {
		cancelRefresh()
		return nil, nil, fmt.Errorf("ensure state dir: %w", err)
	}
	st, err := krb5sync.Load(cfg.Sync.StateFile)
	if err != nil {
		cancelRefresh()
		return nil, nil, err
	}
	mem := krb5sync.NewMemState(st)

	rec := krb5sync.NewReconciler(kc, sdk, mem, krb5sync.Config{
		Realm:         cfg.Krb5.Realm,
		PollInterval:  cfg.Sync.PollInterval,
		EventInterval: cfg.Sync.EventInterval,
		Logger:        logger,
	})

	cleanup := func() {
		cancelRefresh()
	}
	return rec, cleanup, nil
}

func readOptionalFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

// saveState reads the reconciler's MemState snapshot and persists it.
func saveState(path string, rec *krb5sync.Reconciler) error {
	if rec == nil || rec.State == nil {
		return nil
	}
	return krb5sync.Save(path, rec.State.Snapshot())
}

// sdNotify writes a single sd_notify message to $NOTIFY_SOCKET if set.
// Failure to notify is non-fatal — the daemon still works; systemd will
// just see a slower start.
func sdNotify(msg string, logger *slog.Logger) {
	sock := os.Getenv("NOTIFY_SOCKET")
	if sock == "" {
		return
	}
	addr, err := net.ResolveUnixAddr("unixgram", sock)
	if err != nil {
		logger.Debug("sd_notify resolve failed", "err", err)
		return
	}
	conn, err := net.DialUnix("unixgram", nil, addr)
	if err != nil {
		logger.Debug("sd_notify dial failed", "err", err)
		return
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(msg)); err != nil {
		logger.Debug("sd_notify write failed", "err", err)
	}
}
