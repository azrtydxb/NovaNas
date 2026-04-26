// Command disk-agent is a node-local DaemonSet that discovers physical
// block devices on the host and reconciles them into Disk records via
// the NovaNas API.
//
// Architecture: poll-based, no controller-runtime. Every poll
// interval we:
//   1. Enumerate /sys/block via discoverDevices().
//   2. For each device, derive a stable name (wwn → serial → sha1).
//   3. Upsert via the NovaNas API (`POST /api/v1/disks` /
//      `PATCH /api/v1/disks/:name`).
//
// History: previously this agent wrote Disk CRDs directly to the kube
// API server. The CRD-to-Postgres migration moved business objects
// behind packages/api so validation (no system-disk attach to a pool,
// pool class match) is actually authoritative — kubectl can no longer
// bypass the API. The agent authenticates with its pod's projected
// ServiceAccount JWT; the API verifies it via TokenReview.
//
// Hot-plug: poll cycle picks up newly-attached drives; removed disks
// are not auto-deleted (the API record is the system of record for
// capacity-planning history).
//
// Required mounts (see helm DaemonSet template):
//   /sys           ro  — block-device sysfs
//   /dev/disk      ro  — by-id symlinks for stable identification
//   /host/proc     ro  — host /proc/mounts and /proc/swaps for OS-disk detection
//
// Environment:
//   NODE_NAME              — populated from spec.nodeName via downward API
//   NOVANAS_API_URL        — defaults to https://novanas-api.novanas-system.svc
//   NOVANAS_API_TOKEN_PATH — defaults to in-cluster SA token path
//   NOVANAS_API_CA_PATH    — defaults to in-cluster SA CA path
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
)

var safeNameRE = regexp.MustCompile(`[^a-z0-9-]+`)

func main() {
	var (
		interval time.Duration
		nodeName string
		logLevel string
		runOnce  bool
	)
	flag.DurationVar(&interval, "interval", 30*time.Second, "poll interval between scans")
	flag.StringVar(&nodeName, "node", os.Getenv("NODE_NAME"), "node label written to Disk records")
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

	client, err := newApiClient()
	if err != nil {
		logger.Fatal("api client init failed", zap.Error(err))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tick := func() {
		if err := scanAndReconcile(ctx, client, nodeName, logger); err != nil {
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

func scanAndReconcile(ctx context.Context, client *apiClient, nodeName string, logger *zap.Logger) error {
	devices, err := discoverDevices()
	if err != nil {
		return fmt.Errorf("discover devices: %w", err)
	}
	logger.Debug("scan complete", zap.Int("count", len(devices)))

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

// canonicalName builds a stable name from the most-stable identifier
// the device provides: WWN > serial > sha1(path+model). RFC-1123
// compliant, prefixed `disk-`.
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

// classFromType maps the deviceType to the Disk schema's deviceClass enum.
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

// upsertDisk creates the Disk record if missing, then patches its
// status with the current scan result. We never touch spec — that's
// reserved for admin-driven pool assignment via the SPA.
func upsertDisk(ctx context.Context, client *apiClient, name, node string, d deviceInfo) error {
	devName := strings.TrimPrefix(d.Path, "/dev/")

	existing, err := client.GetDisk(ctx, name)
	if err != nil {
		return fmt.Errorf("get disk: %w", err)
	}

	if existing == nil {
		// Fresh disk — create with metadata + initial status.
		labels := map[string]string{
			"novanas.io/node":       node,
			"novanas.io/dev-name":   devName,
			"novanas.io/managed-by": "disk-agent",
		}
		if d.System {
			labels["novanas.io/system"] = "true"
		}
		annotations := map[string]string{}
		if d.SystemReason != "" {
			annotations["novanas.io/system-reason"] = d.SystemReason
		}
		body := &diskEnvelope{
			Metadata: diskMeta{
				Name:        name,
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: map[string]any{},
			Status: map[string]any{
				"slot":        devName,
				"model":       d.Model,
				"serial":      d.Serial,
				"wwn":         d.Wwn,
				"sizeBytes":   int64(d.SizeBytes),
				"class":       classFromType(d.DeviceType),
				"deviceClass": classFromType(d.DeviceType),
				"state":       "IDENTIFIED",
			},
		}
		if _, err := client.CreateDisk(ctx, body); err != nil {
			return fmt.Errorf("create disk: %w", err)
		}
		return nil
	}

	// Existing disk — refresh status + reconcile system label/annotation.
	patch := map[string]any{
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

	// If status.state is missing on the existing record (legacy data),
	// initialise it to IDENTIFIED. We never overwrite existing state
	// values — the operator owns the state machine.
	if getString(existing.Status, "state") == "" {
		patch["status"].(map[string]any)["state"] = "IDENTIFIED"
	}

	// Reconcile system label/annotation. JSON-merge-patch null deletes.
	want := ""
	if d.System {
		want = "true"
	}
	have := existing.Metadata.Labels["novanas.io/system"]
	if have != want {
		labels := map[string]any{}
		if want != "" {
			labels["novanas.io/system"] = want
		} else {
			labels["novanas.io/system"] = nil // delete
		}
		md := map[string]any{"labels": labels}
		if d.SystemReason != "" {
			md["annotations"] = map[string]any{
				"novanas.io/system-reason": d.SystemReason,
			}
		}
		patch["metadata"] = md
	}

	if _, err := client.PatchDisk(ctx, name, patch); err != nil {
		return fmt.Errorf("patch disk: %w", err)
	}
	return nil
}

// getString walks a nested map using string keys.
func getString(m map[string]any, path ...string) string {
	cur := any(m)
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[k]
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}
