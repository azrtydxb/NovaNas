// Package agent: BackendAssignment reconciler driven by the NovaNas API
// (Postgres-backed) instead of a Kubernetes CRD.
//
// Flow:
//  1. Poll /api/v1/backend-assignments on a fixed interval.
//  2. For each assignment whose spec.nodeName matches this node, run the
//     reconciliation logic that programs SPDK via the dataplane gRPC.
//  3. PATCH /api/v1/backend-assignments/<name> with the resulting status.
//
// Replaces the previous controller-runtime CRD watch (#70 deleted all
// CRDs; this is the storage half of completing that migration).
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/azrtydxb/novanas/storage/internal/dataplane"
	"github.com/azrtydxb/novanas/storage/internal/disk"
)

// BackendAssignmentReconciler drives BackendAssignment objects through the
// API. One instance runs per agent process.
type BackendAssignmentReconciler struct {
	API      *BAClient
	NodeName string
	NodeUUID string // Persistent storage node UUID for CRUSH placement.
	DPClient *dataplane.Client
	Logger   *zap.Logger
	BaseBdev string // SPDK bdev name to use (must match --spdk-base-bdev).
	// Interval controls the poll loop in Run. Defaults to 10s.
	Interval time.Duration
}

// Run blocks polling the API until ctx is cancelled. Each tick lists all
// BackendAssignments, filters by node, and reconciles each one. List
// failures are logged and retried — they don't kill the loop.
func (r *BackendAssignmentReconciler) Run(ctx context.Context) {
	interval := r.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	r.Logger.Info("BackendAssignment reconciler running",
		zap.String("node", r.NodeName),
		zap.Duration("interval", interval),
	)
	t := time.NewTicker(interval)
	defer t.Stop()
	// Tick once immediately so the agent reconciles current state at startup
	// without waiting for the first interval.
	r.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

func (r *BackendAssignmentReconciler) tick(ctx context.Context) {
	all, err := r.API.List(ctx)
	if err != nil {
		r.Logger.Warn("BackendAssignment list failed", zap.Error(err))
		return
	}
	for i := range all {
		ba := all[i]
		if ba.Spec.NodeName != r.NodeName {
			continue
		}
		if err := r.reconcile(ctx, &ba, all); err != nil {
			r.Logger.Warn("reconcile BackendAssignment failed",
				zap.String("name", ba.Metadata.Name), zap.Error(err))
		}
	}
}

// reconcile drives one BackendAssignment to its desired state. The full
// `all` list is passed so device-exclusivity checks don't refetch.
func (r *BackendAssignmentReconciler) reconcile(
	ctx context.Context,
	ba *BackendAssignment,
	all []BackendAssignment,
) error {
	r.Logger.Info("reconcile BackendAssignment",
		zap.String("name", ba.Metadata.Name),
		zap.String("node", ba.Spec.NodeName),
		zap.String("phase", ba.Status.Phase),
	)

	// Already provisioned: re-verify dataplane state in case the dataplane
	// pod restarted (bdev + chunk store are in-memory).
	if ba.Status.Phase == "Ready" {
		return r.ensureDataplaneState(ctx, ba)
	}

	// Reset Failed → Pending so we retry on the next tick.
	if ba.Status.Phase == "Failed" {
		_, _ = r.API.PatchStatus(ctx, ba.Metadata.Name, BackendAssignmentStatus{
			Phase:   "Pending",
			Message: "retrying after failure",
		})
	}

	// Mark Provisioning before doing work so observers can see we picked it up.
	_, _ = r.API.PatchStatus(ctx, ba.Metadata.Name, BackendAssignmentStatus{
		Phase:   "Provisioning",
		Message: "discovering devices",
	})

	var bdevName, devicePath, pcieAddr string
	var capacity int64

	switch ba.Spec.BackendType {
	case "file":
		bdevName, capacity = r.provisionFileBackend(ba)

	case "raw", "lvm":
		var err error
		bdevName, devicePath, pcieAddr, capacity, err = r.provisionDeviceBackend(ba, all)
		if err != nil {
			if isTransientGRPCError(err) {
				r.Logger.Warn("transient error provisioning backend, will retry",
					zap.String("name", ba.Metadata.Name), zap.Error(err))
				_, _ = r.API.PatchStatus(ctx, ba.Metadata.Name, BackendAssignmentStatus{
					Phase:   "Provisioning",
					Message: fmt.Sprintf("retrying: %v", err),
				})
				return nil
			}
			r.setFailed(ctx, ba, err.Error())
			return nil
		}

	default:
		r.setFailed(ctx, ba, fmt.Sprintf("unknown backend type: %s", ba.Spec.BackendType))
		return nil
	}

	if bdevName == "" {
		r.setFailed(ctx, ba, "no suitable device found")
		return nil
	}

	if _, err := r.DPClient.InitChunkStore(bdevName, r.NodeName); err != nil {
		if !strings.Contains(err.Error(), "already") {
			if isTransientGRPCError(err) {
				r.Logger.Warn("transient error init chunk store, will retry",
					zap.String("name", ba.Metadata.Name), zap.Error(err))
				_, _ = r.API.PatchStatus(ctx, ba.Metadata.Name, BackendAssignmentStatus{
					Phase:   "Provisioning",
					Message: fmt.Sprintf("retrying chunk store: %v", err),
				})
				return nil
			}
			r.setFailed(ctx, ba, fmt.Sprintf("init chunk store on %s: %v", bdevName, err))
			return nil
		}
	}

	final := BackendAssignmentStatus{
		Phase:    "Ready",
		Device:   devicePath,
		PCIeAddr: pcieAddr,
		BdevName: bdevName,
		Capacity: capacity,
	}
	if _, err := r.API.PatchStatus(ctx, ba.Metadata.Name, final); err != nil {
		return fmt.Errorf("patch status Ready: %w", err)
	}

	r.Logger.Info("BackendAssignment provisioned",
		zap.String("name", ba.Metadata.Name),
		zap.String("bdevName", bdevName),
		zap.String("device", devicePath),
		zap.Int64("capacity", capacity),
	)
	return nil
}

// ensureDataplaneState re-runs InitBackend + InitChunkStore for an already
// Ready assignment. Both calls are idempotent on the dataplane side; this
// catches the case where the dataplane pod restarted.
func (r *BackendAssignmentReconciler) ensureDataplaneState(_ context.Context, ba *BackendAssignment) error {
	bdevName := ba.Status.BdevName
	if bdevName == "" {
		bdevName = r.BaseBdev
	}

	r.Logger.Debug("ensureDataplaneState: verifying backend and chunk store",
		zap.String("name", ba.Metadata.Name),
		zap.String("backendType", ba.Spec.BackendType),
		zap.String("bdevName", bdevName),
		zap.String("device", ba.Status.Device),
	)

	switch ba.Spec.BackendType {
	case "file":
		path := "/var/lib/novanas/file"
		if ba.Spec.FileBackend != nil && ba.Spec.FileBackend.Path != "" {
			path = ba.Spec.FileBackend.Path
		}
		var capacityBytes int64 = 100 * 1024 * 1024 * 1024
		if ba.Spec.FileBackend != nil && ba.Spec.FileBackend.SizeBytes > 0 {
			capacityBytes = ba.Spec.FileBackend.SizeBytes
		}
		cfg := map[string]interface{}{
			"path":           path,
			"capacity_bytes": capacityBytes,
			"name":           bdevName,
		}
		j, _ := json.Marshal(cfg)
		if err := r.DPClient.InitBackend("file", string(j)); err != nil {
			if !strings.Contains(err.Error(), "already") {
				r.Logger.Debug("ensureDataplaneState: file init", zap.Error(err))
			}
		}

	case "raw":
		if ba.Status.Device == "" {
			return nil
		}
		cfg := map[string]interface{}{
			"device_path": ba.Status.Device,
			"name":        bdevName,
		}
		j, _ := json.Marshal(cfg)
		if err := r.DPClient.InitBackend("raw", string(j)); err != nil {
			if !strings.Contains(err.Error(), "already") && !strings.Contains(err.Error(), "returned null") {
				r.Logger.Debug("ensureDataplaneState: raw init", zap.Error(err))
			}
		}

	case "lvm":
		if ba.Status.Device == "" {
			return nil
		}
		raw := map[string]interface{}{
			"device_path": ba.Status.Device,
			"name":        bdevName,
		}
		rj, _ := json.Marshal(raw)
		if err := r.DPClient.InitBackend("raw", string(rj)); err != nil {
			if !strings.Contains(err.Error(), "already") {
				r.Logger.Debug("ensureDataplaneState: lvm raw init", zap.Error(err))
			}
		}
		lvs := fmt.Sprintf("lvs_%s", ba.Spec.PoolRef)
		lvm := map[string]interface{}{
			"bdev_name":    bdevName,
			"lvs_name":     lvs,
			"cluster_size": 1048576,
		}
		lj, _ := json.Marshal(lvm)
		if err := r.DPClient.InitBackend("lvm", string(lj)); err != nil {
			if !strings.Contains(err.Error(), "already") {
				r.Logger.Debug("ensureDataplaneState: lvm init", zap.Error(err))
			}
		}
	}

	if _, err := r.DPClient.InitChunkStore(bdevName, r.NodeName); err != nil {
		if !strings.Contains(err.Error(), "already") {
			r.Logger.Debug("ensureDataplaneState: chunk store init", zap.Error(err))
		}
	}
	return nil
}

func (r *BackendAssignmentReconciler) provisionFileBackend(ba *BackendAssignment) (string, int64) {
	path := "/var/lib/novanas/file"
	if ba.Spec.FileBackend != nil && ba.Spec.FileBackend.Path != "" {
		path = ba.Spec.FileBackend.Path
	}
	var capacityBytes int64 = 100 * 1024 * 1024 * 1024
	if ba.Spec.FileBackend != nil && ba.Spec.FileBackend.SizeBytes > 0 {
		capacityBytes = ba.Spec.FileBackend.SizeBytes
	}
	bdevName := r.BaseBdev
	cfg := map[string]interface{}{
		"path":           path,
		"capacity_bytes": capacityBytes,
		"name":           bdevName,
	}
	j, _ := json.Marshal(cfg)
	if err := r.DPClient.InitBackend("file", string(j)); err != nil {
		r.Logger.Error("failed to init file backend", zap.Error(err))
		return "", 0
	}
	return bdevName, capacityBytes
}

func (r *BackendAssignmentReconciler) provisionDeviceBackend(
	ba *BackendAssignment,
	all []BackendAssignment,
) (bdevName, devicePath, pcieAddr string, capacity int64, err error) {
	devices, discErr := disk.DiscoverDevices()
	if discErr != nil {
		return "", "", "", 0, fmt.Errorf("device discovery: %w", discErr)
	}

	filterOpts := disk.FilterOptions{}
	if ba.Spec.DeviceFilter != nil {
		switch ba.Spec.DeviceFilter.PreferredClass {
		case "nvme":
			filterOpts.DeviceType = disk.TypeNVMe
		case "ssd":
			filterOpts.DeviceType = disk.TypeSSD
		case "hdd":
			filterOpts.DeviceType = disk.TypeHDD
		}
		if ba.Spec.DeviceFilter.MinSize != "" {
			if minBytes, ok := parseBytes(ba.Spec.DeviceFilter.MinSize); ok {
				filterOpts.MinSizeBytes = uint64(minBytes)
			}
		}
	}
	filtered := disk.FilterDevices(devices, filterOpts)
	if len(filtered) == 0 {
		preferred := ""
		if ba.Spec.DeviceFilter != nil {
			preferred = ba.Spec.DeviceFilter.PreferredClass
		}
		return "", "", "", 0, fmt.Errorf("no devices match filter (preferredClass=%s)", preferred)
	}

	// Device exclusivity: skip devices already taken by another BA on this node.
	used := make(map[string]bool)
	for i := range all {
		other := all[i]
		if other.Spec.NodeName != r.NodeName {
			continue
		}
		if other.Metadata.Name == ba.Metadata.Name {
			continue
		}
		if other.Status.Device != "" {
			used[other.Status.Device] = true
		}
	}

	var chosen *disk.DeviceInfo
	for i := range filtered {
		if !used[filtered[i].Path] {
			chosen = &filtered[i]
			break
		}
	}
	if chosen == nil {
		return "", "", "", 0, fmt.Errorf("all matching devices are already assigned to other pools")
	}

	devicePath = chosen.Path
	capacity = int64(chosen.SizeBytes)
	if chosen.DeviceType == disk.TypeNVMe {
		pcieAddr = readNVMePCIeAddr(chosen.Path)
	}

	backendName := r.BaseBdev

	switch ba.Spec.BackendType {
	case "raw":
		cfg := map[string]interface{}{
			"device_path": devicePath,
			"name":        backendName,
		}
		j, _ := json.Marshal(cfg)
		if initErr := r.DPClient.InitBackend("raw", string(j)); initErr != nil {
			msg := initErr.Error()
			if !strings.Contains(msg, "already") && !strings.Contains(msg, "returned null") {
				return "", "", "", 0, fmt.Errorf("init raw backend: %w", initErr)
			}
			r.Logger.Info("raw backend already exists, reusing",
				zap.String("name", backendName), zap.String("device", devicePath))
		}
		bdevName = backendName

	case "lvm":
		raw := map[string]interface{}{
			"device_path": devicePath,
			"name":        backendName,
		}
		rj, _ := json.Marshal(raw)
		if initErr := r.DPClient.InitBackend("raw", string(rj)); initErr != nil {
			if !strings.Contains(initErr.Error(), "already") {
				return "", "", "", 0, fmt.Errorf("attach NVMe for lvm: %w", initErr)
			}
		}
		lvsName := fmt.Sprintf("lvs_%s", ba.Spec.PoolRef)
		lvm := map[string]interface{}{
			"bdev_name":    backendName,
			"lvs_name":     lvsName,
			"cluster_size": 1048576,
		}
		lj, _ := json.Marshal(lvm)
		if initErr := r.DPClient.InitBackend("lvm", string(lj)); initErr != nil {
			return "", "", "", 0, fmt.Errorf("init lvm backend: %w", initErr)
		}
		bdevName = lvsName
	}

	return bdevName, devicePath, pcieAddr, capacity, nil
}

func (r *BackendAssignmentReconciler) setFailed(ctx context.Context, ba *BackendAssignment, msg string) {
	r.Logger.Error("BackendAssignment failed",
		zap.String("name", ba.Metadata.Name),
		zap.String("reason", msg),
	)
	_, _ = r.API.PatchStatus(ctx, ba.Metadata.Name, BackendAssignmentStatus{
		Phase:   "Failed",
		Message: msg,
	})
}

func readNVMePCIeAddr(devPath string) string {
	devName := strings.TrimPrefix(devPath, "/dev/")
	addrPath := fmt.Sprintf("/sys/block/%s/device/address", devName)
	data, err := os.ReadFile(addrPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func isTransientGRPCError(err error) bool {
	if err == nil {
		return false
	}
	s, ok := status.FromError(err)
	if ok {
		switch s.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Aborted, codes.ResourceExhausted:
			return true
		}
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "Unavailable") ||
		strings.Contains(msg, "transport is closing")
}

// parseBytes accepts plain decimal byte counts, plus the same SI/binary
// suffixes (Ki, Mi, Gi, Ti, K, M, G, T) that resource.ParseQuantity
// supported. Returns (n, true) on success. Kept inline so this package
// no longer depends on k8s.io/apimachinery.
func parseBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	type unit struct {
		suf string
		mul int64
	}
	units := []unit{
		{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40}, {"Pi", 1 << 50},
		{"K", 1000}, {"M", 1000 * 1000}, {"G", 1000 * 1000 * 1000},
		{"T", 1000 * 1000 * 1000 * 1000}, {"P", 1000 * 1000 * 1000 * 1000 * 1000},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suf) {
			head := strings.TrimSpace(strings.TrimSuffix(s, u.suf))
			n, err := parseInt64(head)
			if err != nil {
				return 0, false
			}
			return n * u.mul, true
		}
	}
	n, err := parseInt64(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
