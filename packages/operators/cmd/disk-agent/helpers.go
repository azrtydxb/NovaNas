package main

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func newLogger(level string) *zap.Logger {
	cfg := zap.NewProductionConfig()
	switch level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	l, err := cfg.Build()
	if err != nil {
		// Falling back to a no-op logger is worse than a panic in this
		// pre-main path — the operator will be flying blind.
		panic(fmt.Sprintf("zap build: %v", err))
	}
	return l
}

func loadKubeConfig(path string) (*rest.Config, error) {
	if path != "" {
		return clientcmd.BuildConfigFromFlags("", path)
	}
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}
	// Allow `--kubeconfig` via the standard ~/.kube/config search path
	// for local dev runs without -kubeconfig.
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	return cc.ClientConfig()
}

// newDiskObject builds an empty Disk resource we can hand to a
// dynamic Create call. We deliberately do NOT prefill spec — that
// belongs to the operator (pool assignment, role).
//
// `system` toggles the novanas.io/system label so the SPA's
// pool-attach picker can filter the disk out on first creation, even
// before the next reconcile pass refreshes the label via
// reconcileSystemLabel().
func newDiskObject(name, node, devName string, system bool) *unstructured.Unstructured {
	labels := map[string]any{
		"novanas.io/node":       node,
		"novanas.io/dev-name":   devName,
		"novanas.io/managed-by": "disk-agent",
	}
	if system {
		labels["novanas.io/system"] = "true"
	}
	obj := &unstructured.Unstructured{}
	obj.SetUnstructuredContent(map[string]any{
		"apiVersion": "novanas.io/v1alpha1",
		"kind":       "Disk",
		"metadata": map[string]any{
			"name":   name,
			"labels": labels,
		},
		"spec": map[string]any{},
	})
	return obj
}

// reconcileSystemLabel ensures novanas.io/system on the Disk reflects
// the current scan result. Called on every poll because mount state
// can change at runtime (admin grows the OS, mounts an extra fs, …).
func reconcileSystemLabel(
	ctx context.Context,
	client dynamic.NamespaceableResourceInterface,
	name string,
	current *unstructured.Unstructured,
	d deviceInfo,
) error {
	want := ""
	if d.System {
		want = "true"
	}
	have := ""
	if current != nil {
		labels, _, _ := unstructured.NestedStringMap(current.Object, "metadata", "labels")
		have = labels["novanas.io/system"]
	}
	if have == want {
		return nil
	}
	patch := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				"novanas.io/system": nilIfEmpty(want),
			},
		},
	}
	if d.SystemReason != "" {
		patch["metadata"].(map[string]any)["annotations"] = map[string]any{
			"novanas.io/system-reason": d.SystemReason,
		}
	}
	body, err := jsonMarshal(patch)
	if err != nil {
		return err
	}
	_, err = client.Patch(ctx, name, types.MergePatchType, body, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patch system label: %w", err)
	}
	return nil
}

// nilIfEmpty returns nil for an empty string so a JSON merge-patch
// removes the label rather than setting it to "".
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func jsonMarshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return b, nil
}

// getString walks a nested map using string keys and returns the
// final string value, or "" if any step is missing or non-string.
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
