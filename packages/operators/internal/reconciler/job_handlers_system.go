// Package reconciler — system-level job handlers.
//
// These handlers bind to the JobConsumer for jobs emitted by E1 on
// behalf of the operator. Only system.checkUpdate is fully
// implemented; the rest return a clean TODO and log a best-effort
// message so operational tooling can tell "scaffolded" apart from
// "broken".
package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// JobKindCheckUpdate polls the update server for the latest available
// version and writes it to the default UpdatePolicy status.
const JobKindCheckUpdate = "system.checkUpdate"

// JobKindSupportBundle collects host-side diagnostics and returns a
// download URL. Stubbed in this pass.
const JobKindSupportBundle = "system.supportBundle"

// JobKindSystemReset triggers a factory-reset flow. Stubbed — requires
// destructive storage-engine integration.
const JobKindSystemReset = "system.reset"

// JobKindSnapshotRestore creates a BlockVolume populated from a
// snapshot. Scaffolded — full restore requires chunk-engine work.
const JobKindSnapshotRestore = "snapshot.restore"

// updateServerResponse matches the minimum document shape the update
// server is expected to return. Extra fields are ignored.
type updateServerResponse struct {
	LatestVersion string `json:"latestVersion"`
	Channel       string `json:"channel"`
	URL           string `json:"url"`
}

// CheckUpdateHandler returns a JobHandler for system.checkUpdate. The
// handler performs an HTTP GET against UPDATE_SERVER_URL (falling
// back to a well-known default), parses the response, and writes the
// latest version to the named UpdatePolicy's status.
//
// The UpdatePolicy name is taken from the job Input ("policyName"),
// defaulting to "default".
func CheckUpdateHandler(c client.Client, log logr.Logger, httpClient *http.Client) JobHandler {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return func(ctx context.Context, job JobRecord) JobResult {
		policyName, _ := job.Input["policyName"].(string)
		if policyName == "" {
			policyName = "default"
		}
		channel, _ := job.Input["channel"].(string)
		if channel == "" {
			channel = "stable"
		}

		base := os.Getenv("UPDATE_SERVER_URL")
		if base == "" {
			base = "https://updates.novanas.io"
		}
		url := strings.TrimRight(base, "/") + "/v1/latest?channel=" + channel

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return JobResult{Success: false, Message: "build request: " + err.Error()}
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return JobResult{Success: false, Message: "update server unreachable: " + err.Error()}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return JobResult{Success: false, Message: fmt.Sprintf("update server returned %d", resp.StatusCode)}
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return JobResult{Success: false, Message: "read response: " + err.Error()}
		}
		var parsed updateServerResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return JobResult{Success: false, Message: "parse response: " + err.Error()}
		}
		if parsed.LatestVersion == "" {
			return JobResult{Success: false, Message: "update server returned no version"}
		}

		// Write to the UpdatePolicy status via unstructured merge
		// patch so we don't need to evolve the typed Status struct.
		gvk := schema.GroupVersionKind{Group: "novanas.io", Version: "v1alpha1", Kind: "UpdatePolicy"}
		cur := &unstructured.Unstructured{}
		cur.SetGroupVersionKind(gvk)
		if getErr := c.Get(ctx, types.NamespacedName{Name: policyName}, cur); getErr != nil {
			if apierrors.IsNotFound(getErr) {
				// Create a minimal policy so the check isn't lost.
				cur.SetName(policyName)
				_ = c.Create(ctx, cur)
			} else {
				log.V(1).Info("update policy fetch failed", "error", getErr.Error())
			}
		}
		patched := cur.DeepCopy()
		status := map[string]any{
			"phase":              "Ready",
			"latestVersion":      parsed.LatestVersion,
			"downloadURL":        parsed.URL,
			"lastCheckTimestamp": metav1.Now().UTC().Format(time.RFC3339),
		}
		_ = unstructured.SetNestedMap(patched.Object, status, "status")
		if err := c.Status().Patch(ctx, patched, client.MergeFrom(cur)); err != nil {
			// Non-fatal: we still return the version in the job
			// result so the UI has something to show.
			log.V(1).Info("update policy status write failed", "error", err.Error())
		}

		return JobResult{
			Success: true,
			Message: "latest version: " + parsed.LatestVersion,
			Result: map[string]any{
				"latestVersion": parsed.LatestVersion,
				"channel":       channel,
				"url":           parsed.URL,
			},
		}
	}
}

// SupportBundleHandler stubs system.supportBundle. A real
// implementation would create a privileged Pod that gathers logs /
// sosreport output into a PVC and exposes it via a signed URL.
// TODO(operators): wire support-bundle Pod + signed URL generation.
func SupportBundleHandler(log logr.Logger) JobHandler {
	return func(ctx context.Context, job JobRecord) JobResult {
		log.Info("system.supportBundle invoked; not yet fully implemented — returning stub URL", "jobID", job.ID)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return JobResult{Success: false, Message: "cancelled"}
		}
		return JobResult{
			Success: true,
			Message: "support bundle stubbed (no real bundle produced)",
			Result: map[string]any{
				"downloadURL": "https://support-bundle.invalid/" + job.ID + ".tar.gz",
				"stub":        true,
			},
		}
	}
}

// SystemResetHandler stubs system.reset. A real implementation would
// drain workloads, wipe storage pools, and reboot the node.
// TODO(operators): wire real reset path once destructive storage
// operations are available.
func SystemResetHandler(log logr.Logger) JobHandler {
	return func(_ context.Context, job JobRecord) JobResult {
		log.Info("system.reset invoked; not yet implemented — refusing to no-op silently", "jobID", job.ID)
		return JobResult{
			Success: false,
			Message: "system.reset not yet implemented: destructive storage ops unavailable",
		}
	}
}

// SnapshotRestoreHandler scaffolds snapshot.restore by creating a
// BlockVolume that references the source snapshot. The backing
// chunk-engine restore is still a TODO.
// TODO(operators): wire chunk-engine restore into the volume's
// populator.
func SnapshotRestoreHandler(c client.Client, log logr.Logger) JobHandler {
	return func(ctx context.Context, job JobRecord) JobResult {
		snapName, _ := job.Input["snapshotName"].(string)
		targetVolume, _ := job.Input["targetVolume"].(string)
		ns, _ := job.Input["namespace"].(string)
		if ns == "" {
			ns = "novanas-system"
		}
		if snapName == "" || targetVolume == "" {
			return JobResult{Success: false, Message: "snapshot.restore requires snapshotName + targetVolume"}
		}
		gvk := schema.GroupVersionKind{Group: "novanas.io", Version: "v1alpha1", Kind: "BlockVolume"}
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		u.SetName(targetVolume)
		u.SetNamespace(ns)
		spec := map[string]any{
			"restoreFromSnapshot": snapName,
		}
		_ = unstructured.SetNestedMap(u.Object, spec, "spec")
		if err := c.Create(ctx, u); err != nil && !apierrors.IsAlreadyExists(err) {
			return JobResult{Success: false, Message: "create target volume: " + err.Error()}
		}
		log.Info("snapshot.restore: target BlockVolume created; chunk-engine restore not yet implemented",
			"snapshot", snapName, "target", targetVolume)
		return JobResult{
			Success: true,
			Message: "restore scaffolded; target volume created, chunk-engine restore pending",
			Result: map[string]any{
				"targetVolume": targetVolume,
				"namespace":    ns,
				"stub":         true,
			},
		}
	}
}
