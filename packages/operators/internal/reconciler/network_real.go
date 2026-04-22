// Package reconciler — NetworkClient backed by a Kubernetes ConfigMap.
//
// For v1 the operator writes the rendered nmstate YAML into a ConfigMap
// named "novanas-netstate-<node>" in the operator namespace (default
// "novanas-system"). A separate cluster-wide nmstate-applier DaemonSet
// (installed by the OS wave) watches these ConfigMaps on its node and
// applies the state via `nmstatectl apply`. This keeps the operator
// pod sandboxed — it never shells out to nmstatectl itself.
package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMapNetworkClient materialises nmstate YAML into per-node
// ConfigMaps that the nmstate-applier DaemonSet reads.
type ConfigMapNetworkClient struct {
	// Client is the controller-runtime client used to write ConfigMaps.
	Client client.Client
	// Namespace is the operator namespace, default "novanas-system".
	Namespace string
}

// NewConfigMapNetworkClient constructs a NetworkClient. namespace
// defaults to "novanas-system" if empty.
func NewConfigMapNetworkClient(c client.Client, namespace string) *ConfigMapNetworkClient {
	if namespace == "" {
		namespace = "novanas-system"
	}
	return &ConfigMapNetworkClient{Client: c, Namespace: namespace}
}

// ApplyState upserts a ConfigMap carrying the nmstate YAML for `node`.
// Revision is the sha256 of the YAML (stable input->output mapping)
// which applier DaemonSets use to detect changes.
func (n *ConfigMapNetworkClient) ApplyState(ctx context.Context, node string, stateYAML []byte) (string, error) {
	if n == nil || n.Client == nil {
		return "", errors.New("network client: not configured")
	}
	if node == "" {
		return "", errors.New("network client: empty node name")
	}
	cmName := configMapNameForNode(node)
	sum := sha256.Sum256(stateYAML)
	rev := hex.EncodeToString(sum[:])

	cm := &corev1.ConfigMap{}
	err := n.Client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: n.Namespace}, cm)
	if apierrors.IsNotFound(err) {
		newCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: n.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "novanas-operators",
					"novanas.io/netstate-node":     node,
				},
				Annotations: map[string]string{
					"novanas.io/netstate-revision": rev,
				},
			},
			Data: map[string]string{
				"nmstate.yaml": string(stateYAML),
			},
		}
		if cerr := n.Client.Create(ctx, newCM); cerr != nil {
			return "", fmt.Errorf("network client: create configmap: %w", cerr)
		}
		return rev, nil
	} else if err != nil {
		return "", fmt.Errorf("network client: get configmap: %w", err)
	}

	// Update existing.
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["nmstate.yaml"] = string(stateYAML)
	if cm.Annotations == nil {
		cm.Annotations = map[string]string{}
	}
	cm.Annotations["novanas.io/netstate-revision"] = rev
	if uerr := n.Client.Update(ctx, cm); uerr != nil {
		return "", fmt.Errorf("network client: update configmap: %w", uerr)
	}
	return rev, nil
}

// ObservedState reads the last-applied YAML back from the ConfigMap.
// Returns nil when the ConfigMap does not exist yet.
func (n *ConfigMapNetworkClient) ObservedState(ctx context.Context, node string) ([]byte, error) {
	if n == nil || n.Client == nil {
		return nil, nil
	}
	cm := &corev1.ConfigMap{}
	err := n.Client.Get(ctx, types.NamespacedName{Name: configMapNameForNode(node), Namespace: n.Namespace}, cm)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("network client: observed: %w", err)
	}
	return []byte(cm.Data["nmstate.yaml"]), nil
}

func configMapNameForNode(node string) string {
	// ConfigMap names must be DNS1123; nodes are already valid.
	return "novanas-netstate-" + node
}

var _ NetworkClient = (*ConfigMapNetworkClient)(nil)
