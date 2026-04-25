// Command disk-agent is a node-local DaemonSet that discovers physical
// block devices on the host and reconciles them into Disk CRs in the
// novanas.io API group.
//
// Architecture: poll-based, no controller-runtime. Every poll
// interval we:
//   1. Enumerate /sys/block via storage/internal/disk.DiscoverDevices.
//   2. For each device, derive a stable name (wwn → serial-hash →
//      model+path hash) suitable for K8s metadata.name.
//   3. Upsert the corresponding Disk CR. Status fields (model,
//      serial, wwn, sizeBytes, deviceClass) are written via the
//      status sub-resource. State stays at IDENTIFIED until an admin
//      assigns the disk to a pool (sets spec.pool / spec.role).
//
// Hot-plug: we currently rely on the poll cycle (default 30s) to
// pick up newly-attached drives. Removed disks transition to
// REMOVABLE on the next pass and are not deleted (the Disk CR is
// the system-of-record for capacity-planning history).
//
// Required mounts (see helm DaemonSet template):
//   /sys           ro  — block-device sysfs
//   /dev/disk      ro  — by-id symlinks for stable identification
//
// Required RBAC (see helm/templates/disk-agent/rbac.yaml):
//   create/get/list/update/patch  on  disks.novanas.io
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

var diskGVR = schema.GroupVersionResource{
	Group:    "novanas.io",
	Version:  "v1alpha1",
	Resource: "disks",
}

var safeNameRE = regexp.MustCompile(`[^a-z0-9-]+`)

func main() {
	var (
		interval  time.Duration
		nodeName  string
		kubecfg   string
		logLevel  string
		runOnce   bool
	)
	flag.DurationVar(&interval, "interval", 30*time.Second, "poll interval between scans")
	flag.StringVar(&nodeName, "node", os.Getenv("NODE_NAME"), "node label written to Disk CRs")
	flag.StringVar(&kubecfg, "kubeconfig", "", "path to kubeconfig (empty = in-cluster)")
	flag.StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
	flag.BoolVar(&runOnce, "once", false, "scan once and exit (for jobs)")
	flag.Parse()

	logger := newLogger(logLevel)
	defer func() { _ = logger.Sync() }()

	if nodeName == "" {
		hn, _ := os.Hostname()
		nodeName = hn
	}
	logger.Info("disk-agent starting", zap.String("node", nodeName), zap.Duration("interval", interval))

	cfg, err := loadKubeConfig(kubecfg)
	if err != nil {
		logger.Fatal("loading kubeconfig", zap.Error(err))
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		logger.Fatal("dynamic client", zap.Error(err))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tick := func() {
		if err := scanAndReconcile(ctx, dyn, nodeName, logger); err != nil {
			logger.Error("scan failed", zap.Error(err))
		}
	}

	tick()
	if runOnce {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("disk-agent stopping")
			return
		case <-t.C:
			tick()
		}
	}
}

func scanAndReconcile(ctx context.Context, dyn dynamic.Interface, nodeName string, logger *zap.Logger) error {
	devices, err := discoverDevices()
	if err != nil {
		return fmt.Errorf("discover devices: %w", err)
	}
	logger.Debug("scan complete", zap.Int("count", len(devices)))

	client := dyn.Resource(diskGVR)
	for _, d := range devices {
		name := canonicalName(d)
		if name == "" {
			logger.Warn("skipping device with no stable identifier", zap.String("path", d.Path))
			continue
		}
		if err := upsertDisk(ctx, client, name, nodeName, d); err != nil {
			logger.Error("upsert disk failed", zap.String("name", name), zap.Error(err))
		}
	}
	return nil
}

// canonicalName builds a K8s-safe metadata.name from the most stable
// identifier the device provides: WWN > serial > sha1(path+model).
//
// K8s names must be lowercase RFC 1123, max 63 chars. We normalise to
// that and prefix with the kind to keep a flat namespace readable.
func canonicalName(d deviceInfo) string {
	var key string
	switch {
	case d.Wwn != "":
		key = strings.TrimPrefix(d.Wwn, "0x")
	case d.Serial != "":
		key = d.Serial
	default:
		h := sha1.Sum([]byte(d.Path + ":" + d.Model))
		key = hex.EncodeToString(h[:8])
	}
	key = strings.ToLower(key)
	key = safeNameRE.ReplaceAllString(key, "-")
	key = strings.Trim(key, "-")
	if len(key) > 56 {
		key = key[:56]
	}
	if key == "" {
		return ""
	}
	return "disk-" + key
}

// classFromType maps the storage/internal/disk type to the Disk CRD
// deviceClass enum (nvme/ssd/hdd).
func classFromType(t deviceType) string {
	switch t {
	case typeNVMe:
		return "nvme"
	case typeSSD:
		return "ssd"
	case typeHDD:
		return "hdd"
	default:
		return ""
	}
}

// upsertDisk creates the Disk CR if missing, then patches its status
// with the current scan result. We never overwrite spec — that's
// reserved for admin-driven pool assignment.
func upsertDisk(ctx context.Context, client dynamic.NamespaceableResourceInterface, name, node string, d deviceInfo) error {
	devName := strings.TrimPrefix(d.Path, "/dev/")

	// 1. Ensure the resource exists.
	current, err := client.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		obj := newDiskObject(name, node, devName, d.System)
		_, err = client.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create disk: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get disk: %w", err)
	}

	// 1b. Reconcile the system-disk label on every pass — the answer
	//     can change at runtime (e.g. user mounts a new filesystem).
	if err := reconcileSystemLabel(ctx, client, name, current, d); err != nil {
		return err
	}

	// 2. Patch status fields with the latest scan result. We use a
	//    JSON-merge patch on the /status sub-resource so the operator
	//    can manage state transitions independently of static facts
	//    like model/serial.
	statusPatch := map[string]any{
		"status": map[string]any{
			"slot":        devName,
			"model":       d.Model,
			"serial":      d.Serial,
			"wwn":         d.Wwn,
			"sizeBytes":   int64(d.SizeBytes),
			"class":       classFromType(d.DeviceType),
			"deviceClass": classFromType(d.DeviceType),
		},
	}
	body, err := jsonMarshal(statusPatch)
	if err != nil {
		return err
	}
	_, err = client.Patch(ctx, name, types.MergePatchType, body, metav1.PatchOptions{}, "status")
	if err != nil {
		return fmt.Errorf("patch disk status: %w", err)
	}

	// 3. If this is a fresh disk, set state=IDENTIFIED so admins see
	//    it ready for assignment. We don't touch state on subsequent
	//    passes — the operator owns that lifecycle.
	if current == nil || getString(current.Object, "status", "state") == "" {
		stateBody, _ := jsonMarshal(map[string]any{
			"status": map[string]any{"state": "IDENTIFIED"},
		})
		_, _ = client.Patch(ctx, name, types.MergePatchType, stateBody, metav1.PatchOptions{}, "status")
	}
	return nil
}
