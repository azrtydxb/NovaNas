// Command manager is the NovaNas controller-manager entrypoint. It boots
// controller-runtime, registers the NovaNas API types with the scheme, and
// wires a no-op reconciler for every CRD kind. Real reconcile logic is
// implemented in Wave 4+.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"

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
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(novanasv1alpha1.AddToScheme(scheme))
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

	if err := setupAllControllers(mgr); err != nil {
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

// setupAllControllers wires every NovaNas reconciler into the manager.
func setupAllControllers(mgr ctrl.Manager) error {
	type setup interface {
		SetupWithManager(mgr ctrl.Manager) error
	}

	reconcilers := []setup{
		&controllers.StoragePoolReconciler{},
		&controllers.BlockVolumeReconciler{},
		&controllers.DatasetReconciler{},
		&controllers.BucketReconciler{},
		&controllers.DiskReconciler{},
		&controllers.ShareReconciler{},
		&controllers.SmbServerReconciler{},
		&controllers.NfsServerReconciler{},
		&controllers.IscsiTargetReconciler{},
		&controllers.NvmeofTargetReconciler{},
		&controllers.ObjectStoreReconciler{},
		&controllers.BucketUserReconciler{},
		&controllers.UserReconciler{},
		&controllers.GroupReconciler{},
		&controllers.KeycloakRealmReconciler{},
		&controllers.ApiTokenReconciler{},
		&controllers.SshKeyReconciler{},
		&controllers.SnapshotReconciler{},
		&controllers.SnapshotScheduleReconciler{},
		&controllers.ReplicationTargetReconciler{},
		&controllers.ReplicationJobReconciler{},
		&controllers.CloudBackupTargetReconciler{},
		&controllers.CloudBackupJobReconciler{},
		&controllers.ScrubScheduleReconciler{},
		&controllers.PhysicalInterfaceReconciler{},
		&controllers.BondReconciler{},
		&controllers.VlanReconciler{},
		&controllers.HostInterfaceReconciler{},
		&controllers.ClusterNetworkReconciler{},
		&controllers.VipPoolReconciler{},
		&controllers.IngressReconciler{},
		&controllers.RemoteAccessTunnelReconciler{},
		&controllers.CustomDomainReconciler{},
		&controllers.FirewallRuleReconciler{},
		&controllers.TrafficPolicyReconciler{},
		&controllers.AppCatalogReconciler{},
		&controllers.AppReconciler{},
		&controllers.AppInstanceReconciler{},
		&controllers.VmReconciler{},
		&controllers.IsoLibraryReconciler{},
		&controllers.GpuDeviceReconciler{},
		&controllers.EncryptionPolicyReconciler{},
		&controllers.KmsKeyReconciler{},
		&controllers.CertificateReconciler{},
		&controllers.SmartPolicyReconciler{},
		&controllers.AlertChannelReconciler{},
		&controllers.AlertPolicyReconciler{},
		&controllers.AuditPolicyReconciler{},
		&controllers.ServiceLevelObjectiveReconciler{},
		&controllers.UpsPolicyReconciler{},
		&controllers.ConfigBackupPolicyReconciler{},
		&controllers.SystemSettingsReconciler{},
		&controllers.UpdatePolicyReconciler{},
		&controllers.ServicePolicyReconciler{},
	}

	for _, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup controller %T: %w", r, err)
		}
	}
	return nil
}
