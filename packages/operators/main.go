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

	"github.com/go-logr/logr"
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
	"github.com/azrtydxb/novanas/packages/operators/internal/vmworker"
	rtk8s "github.com/azrtydxb/novanas/packages/runtime/k8s"
	novanasclient "github.com/azrtydxb/novanas/packages/sdk/go-client"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(novanasv1alpha1.AddToScheme(scheme))
}

// buildKeyProvisioner returns the OpenBao Transit-backed volume key
// provisioner used by BlockVolumeReconciler. Encryption is
// security-critical: refuse to start without a working provisioner
// unless NOVANAS_DEV=1 is explicitly set. A silent no-op would write
// a placeholder wrapped-DK to CR status and permanently prevent later
// recovery of the data.
func buildKeyProvisioner() reconciler.VolumeKeyProvisioner {
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
			setupLog.Info("NOVANAS_DEV=1 set; no key provisioner wired (encrypted resources will fail to reconcile)")
			return nil
		}
		setupLog.Info("transit key provisioner wired (OpenBao Transit)")
		return tp
	}
	if os.Getenv("NOVANAS_DEV") == "1" {
		setupLog.Info("OPENBAO_ADDR not set and NOVANAS_DEV=1; no key provisioner wired (encrypted resources will error)")
		return nil
	}
	setupLog.Error(nil, "OPENBAO_ADDR not set; refusing to start without a volume-key provisioner. Set OPENBAO_ADDR, or NOVANAS_DEV=1 for explicit dev override.")
	os.Exit(1)
	return nil
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

// vmWorkerRunnable adapts the VM worker onto manager.Runnable so it
// shares the manager's leader-election + signal handling.
type vmWorkerRunnable struct {
	worker *vmworker.Worker
}

func (v vmWorkerRunnable) Start(ctx context.Context) error { return v.worker.Start(ctx) }

// buildVmWorker constructs the VM worker if both the API server URL
// and an in-cluster (or KUBECONFIG) k8s config are available. Returns
// nil when either piece is missing — the manager logs and continues.
func buildVmWorker(log logr.Logger) *vmworker.Worker {
	if os.Getenv("NOVANAS_API_URL") == "" {
		return nil
	}
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Error(err, "vm worker: get k8s config")
		return nil
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "vm worker: build kubernetes client")
		return nil
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "vm worker: build dynamic client")
		return nil
	}
	api, err := novanasclient.NewFromEnv()
	if err != nil {
		log.Error(err, "vm worker: build novanas api client")
		return nil
	}
	adapter := rtk8s.New(cs).WithDynamicClient(dyn)
	return &vmworker.Worker{
		Client:   api,
		Adapter:  adapter,
		Interval: 30 * time.Second,
		Log:      log.WithName("vmworker"),
	}
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

	keyProv := buildKeyProvisioner()

	if err := setupAllControllers(mgr, keyProv); err != nil {
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

	// --- VM worker: poll API server, drive runtime.Adapter ---
	if vmw := buildVmWorker(setupLog); vmw != nil {
		if err := mgr.Add(vmWorkerRunnable{worker: vmw}); err != nil {
			setupLog.Error(err, "unable to add vm worker to manager")
			os.Exit(1)
		}
		setupLog.Info("vm worker wired (api -> runtime.Adapter k8s+kubevirt)")
	} else {
		setupLog.Info("vm worker disabled (NOVANAS_API_URL or kubeconfig unavailable)")
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

// setupAllControllers wires every surviving NovaNas reconciler into
// the manager. After PRs 1–11 only BlockVolumeReconciler remains; it
// flips to a runtime.Adapter-driven worker when storage data plane #50
// lands.
func setupAllControllers(mgr ctrl.Manager, keyProv reconciler.VolumeKeyProvisioner) error {
	type setup interface {
		SetupWithManager(mgr ctrl.Manager) error
	}
	reconcilers := []setup{
		&controllers.BlockVolumeReconciler{KeyProvisioner: keyProv},
	}
	for _, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup controller %T: %w", r, err)
		}
	}
	return nil
}
