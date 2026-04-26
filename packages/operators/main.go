// Command manager is the NovaNas controller-manager entrypoint. It boots
// controller-runtime, registers the NovaNas API types with the scheme,
// and wires every reconciler with the production implementations of
// the pluggable client interfaces (Keycloak, storage, certificate
// issuer, network, updater, VM engine, volume-key provisioner).
//
// Each real client is constructed from environment variables. If
// construction fails (missing config, unreachable service) the wiring
// falls back to the corresponding NoopXxx with a loud warning so the
// operator still starts — controllers keep running and will surface
// the degraded state via conditions on individual CRs.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/controllers"
	"github.com/azrtydxb/novanas/packages/operators/internal/logging"
	_ "github.com/azrtydxb/novanas/packages/operators/internal/metrics"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(novanasv1alpha1.AddToScheme(scheme))
}

// externalClients aggregates the production client implementations
// injected into reconcilers. Each field is guaranteed non-nil — failed
// constructions fall back to their NoopXxx counterparts so reconcilers
// never panic on a nil dependency.
type externalClients struct {
	keycloak   reconciler.KeycloakClient
	storage    reconciler.StorageClient
	certIssuer reconciler.CertificateIssuer
	network    reconciler.NetworkClient
	updater    reconciler.UpdateClient
	vmEngine   reconciler.VmEngine
	keyProv    reconciler.VolumeKeyProvisioner
	openbao    reconciler.OpenBaoClient
}

func buildExternalClients(mgr ctrl.Manager) externalClients {
	ec := externalClients{
		keycloak:   reconciler.NoopKeycloakClient{},
		storage:    reconciler.NoopStorageClient{},
		certIssuer: reconciler.NoopCertificateIssuer{},
		network:    reconciler.NoopNetworkClient{},
		updater:    reconciler.NoopUpdateClient{},
		vmEngine:   reconciler.NoopVmEngine{},
		// keyProv intentionally starts nil: populated only when a real
		// provisioner is wired, or when NOVANAS_DEV=1 overrides.
		keyProv: nil,
		openbao:    reconciler.NoopOpenBaoClient{},
	}

	// --- Keycloak ---
	if addr := os.Getenv("KEYCLOAK_URL"); addr != "" {
		kc, err := reconciler.NewGocloakClient(reconciler.GocloakConfig{
			BaseURL:      addr,
			AdminRealm:   envDefault("KEYCLOAK_ADMIN_REALM", "master"),
			ClientID:     os.Getenv("KEYCLOAK_ADMIN_CLIENT_ID"),
			ClientSecret: os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		})
		if err != nil {
			setupLog.Error(err, "keycloak client init failed; falling back to no-op")
		} else {
			setupLog.Info("keycloak client wired", "addr", addr)
			ec.keycloak = kc
		}
	} else {
		setupLog.Info("KEYCLOAK_URL not set; using no-op keycloak client (dev mode)")
	}

	// --- Storage gRPC client ---
	{
		addr := envDefault("STORAGE_META_ADDR", "novanas-storage-meta.novanas-system.svc:7001")
		sc, err := reconciler.NewGRPCStorageClient(reconciler.GRPCStorageClientConfig{
			Address:     addr,
			CAFile:      os.Getenv("STORAGE_TLS_CA"),
			CertFile:    os.Getenv("STORAGE_TLS_CERT"),
			KeyFile:     os.Getenv("STORAGE_TLS_KEY"),
			ServerName:  os.Getenv("STORAGE_TLS_SERVER_NAME"),
			DialTimeout: 10 * time.Second,
			CallTimeout: 15 * time.Second,
		})
		if err != nil {
			setupLog.Error(err, "storage client init failed; falling back to no-op", "addr", addr)
		} else {
			setupLog.Info("storage gRPC client wired", "addr", addr)
			ec.storage = sc
		}
	}

	// --- Certificate issuer (OpenBao PKI) ---
	if addr := os.Getenv("OPENBAO_ADDR"); addr != "" {
		issuer, err := reconciler.NewOpenBaoPKIIssuer(reconciler.OpenBaoPKIConfig{
			Addr:      addr,
			Token:     os.Getenv("OPENBAO_TOKEN"),
			TokenPath: envDefault("OPENBAO_TOKEN_PATH", "/var/run/secrets/openbao/token"),
			Namespace: os.Getenv("OPENBAO_NAMESPACE"),
			MountPath: envDefault("OPENBAO_PKI_PATH", "pki"),
			Role:      envDefault("OPENBAO_PKI_ROLE", "novanas"),
		})
		if err != nil {
			setupLog.Error(err, "openbao PKI issuer init failed; falling back to no-op")
		} else {
			setupLog.Info("openbao PKI issuer wired", "addr", addr)
			ec.certIssuer = issuer
		}
	} else {
		setupLog.Info("OPENBAO_ADDR not set; using no-op certificate issuer (dev mode)")
	}

	// --- Network client (ConfigMap projection) ---
	ns := envDefault("OPERATOR_NAMESPACE", "novanas-system")
	ec.network = reconciler.NewConfigMapNetworkClient(mgr.GetClient(), ns)
	setupLog.Info("network client wired (ConfigMap projection)", "namespace", ns)

	// --- Update client (ConfigMap-driven host updater) ---
	ec.updater = reconciler.NewConfigMapUpdateClient(mgr.GetClient(), ns)
	setupLog.Info("update client wired (ConfigMap host-updater bridge)", "namespace", ns)

	// --- VM engine (KubeVirt via unstructured) ---
	ec.vmEngine = reconciler.NewKubeVirtEngine(mgr.GetClient())
	setupLog.Info("kubevirt engine wired (unstructured VirtualMachine client)")

	// --- Volume-key provisioner ---
	//
	// Encryption is security-critical: refuse to start with a NoopKeyProvisioner
	// unless NOVANAS_DEV=1 is explicitly set. A silent noop would write a
	// placeholder wrapped-DK to CR status and permanently prevent later
	// recovery of the data.
	if os.Getenv("OPENBAO_ADDR") != "" {
		tp, err := reconciler.NewTransitKeyProvisioner(reconciler.TransitKeyProvisionerConfig{
			Addr:          os.Getenv("OPENBAO_ADDR"),
			Token:         os.Getenv("OPENBAO_TOKEN"),
			TokenPath:     envDefault("OPENBAO_TOKEN_PATH", "/var/run/secrets/openbao/token"),
			Namespace:     os.Getenv("OPENBAO_NAMESPACE"),
			MountPath:     envDefault("OPENBAO_TRANSIT_PATH", "transit"),
			MasterKeyName: envDefault("OPENBAO_MASTER_KEY", "novanas/chunk-master"),
		})
		if err != nil {
			setupLog.Error(err, "transit key provisioner init failed")
			if os.Getenv("NOVANAS_DEV") != "1" {
				setupLog.Error(err, "refusing to start without a working key provisioner (set NOVANAS_DEV=1 for dev override)")
				os.Exit(1)
			}
			setupLog.Info("NOVANAS_DEV=1 set; using no-op key provisioner (dev only; encrypted resources will fail to reconcile)")
			ec.keyProv = nil
		} else {
			setupLog.Info("transit key provisioner wired (OpenBao Transit)")
			ec.keyProv = tp
		}
	} else if os.Getenv("NOVANAS_DEV") == "1" {
		setupLog.Info("OPENBAO_ADDR not set and NOVANAS_DEV=1; no key provisioner wired (encrypted resources will error)")
		ec.keyProv = nil
	} else {
		setupLog.Error(nil, "OPENBAO_ADDR not set; refusing to start without a volume-key provisioner. Set OPENBAO_ADDR, or NOVANAS_DEV=1 for explicit dev override.")
		os.Exit(1)
	}

	// --- OpenBao admin client (policies + kubernetes-auth roles) ---
	if addr := os.Getenv("OPENBAO_ADDR"); addr != "" {
		token := os.Getenv("OPENBAO_TOKEN")
		if token == "" {
			// Best-effort: let the client read OPENBAO_TOKEN_PATH on each call.
			// If neither is set the constructor will return an error.
			if tp := envDefault("OPENBAO_TOKEN_PATH", ""); tp == "" {
				setupLog.Info("OPENBAO_TOKEN and OPENBAO_TOKEN_PATH not set; using no-op openbao admin client")
			}
		}
		obc, err := reconciler.NewHTTPOpenBaoClient(addr, token)
		if err != nil {
			setupLog.Error(err, "openbao admin client init failed; falling back to no-op")
		} else {
			setupLog.Info("openbao admin client wired", "addr", addr)
			ec.openbao = obc
		}
	} else {
		setupLog.Info("OPENBAO_ADDR not set; using no-op openbao admin client (dev mode)")
	}

	return ec
}

// jobConsumerRunnable adapts a *JobConsumer to controller-runtime's
// manager.Runnable interface so the consumer's poll goroutines start
// when the manager starts and stop when ctx is cancelled.
type jobConsumerRunnable struct {
	consumer *reconciler.JobConsumer
}

// Start launches the consumer's poll goroutines and blocks on ctx.
func (j jobConsumerRunnable) Start(ctx context.Context) error {
	if err := j.consumer.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
		enableHTTP2          bool
		development          bool
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election. Only one manager with this enabled becomes active at a time.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"Enable HTTP/2 for the webhook and metrics servers (off by default for CVE-2023-44487).")
	flag.BoolVar(&development, "development", false, "Enable human-friendly development logging.")
	flag.Parse()

	ctrl.SetLogger(logging.New(development))

	// HTTP/2 is disabled by default to avoid CVE-2023-44487 / -39325. Operators
	// that actually need HTTP/2 opt in with --enable-http2.
	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) { c.NextProtos = []string{"http/1.1"} })
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
			TLSOpts:     tlsOpts,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			TLSOpts: tlsOpts,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "novanas-operators.novanas.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	clients := buildExternalClients(mgr)

	if err := setupAllControllers(mgr, clients); err != nil {
		setupLog.Error(err, "unable to set up controllers")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// --- JobConsumer: system-level jobs from E1 -----------------------
	// The jobs backend is left as a NoopJobsBackend in this pass —
	// the operator is ready to consume jobs, but the bridge to the
	// NovaFlow API jobs table is a follow-up (F3). Handlers below
	// are still wired so the dispatch table compiles and so a future
	// backend swap flips the feature on with zero controller
	// changes.
	jobConsumer := reconciler.NewJobConsumer(reconciler.NoopJobsBackend{}, ctrl.Log.WithName("jobconsumer"))
	jobConsumer.Register(reconciler.JobKindCheckUpdate, reconciler.CheckUpdateHandler(mgr.GetClient(), ctrl.Log.WithName("checkupdate"), nil))
	jobConsumer.Register(reconciler.JobKindSupportBundle, reconciler.SupportBundleHandler(ctrl.Log.WithName("supportbundle")))
	jobConsumer.Register(reconciler.JobKindSystemReset, reconciler.SystemResetHandler(ctrl.Log.WithName("systemreset")))
	jobConsumer.Register(reconciler.JobKindSnapshotRestore, reconciler.SnapshotRestoreHandler(mgr.GetClient(), ctrl.Log.WithName("snapshotrestore")))
	if err := mgr.Add(jobConsumerRunnable{consumer: jobConsumer}); err != nil {
		setupLog.Error(err, "unable to add job consumer to manager")
		os.Exit(1)
	}

	setupLog.Info("starting manager",
		"metricsAddr", metricsAddr,
		"probeAddr", probeAddr,
		"leaderElect", enableLeaderElection,
	)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// setupAllControllers wires every NovaNas reconciler into the manager,
// injecting the external clients into reconcilers that need them.
func setupAllControllers(mgr ctrl.Manager, ec externalClients) error {
	type setup interface {
		SetupWithManager(mgr ctrl.Manager) error
	}

	// CRD-to-Postgres migration: most business-object reconcilers
	// have been removed. FirewallRule, TrafficPolicy and ServicePolicy
	// joined the grey set in this PR — flipped to PgResource; the
	// novanet/host-agent consumer becomes an API client in a follow-up.
	// What remains are reconcilers that produce real runtime objects:
	// VMs (KubeVirt), AppInstance (Helm), Ingress and
	// RemoteAccessTunnel (novaedge), and BlockVolume (storage data
	// plane — flips with #50).
	reconcilers := []setup{
		&controllers.BlockVolumeReconciler{KeyProvisioner: ec.keyProv},
		&controllers.IngressReconciler{},
		&controllers.RemoteAccessTunnelReconciler{},
		&controllers.AppReconciler{},
		&controllers.AppInstanceReconciler{},
		&controllers.VmReconciler{Engine: ec.vmEngine},
		&controllers.GpuDeviceReconciler{},
	}

	for _, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup controller %T: %w", r, err)
		}
	}
	return nil
}
