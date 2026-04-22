// Package reconciler — UpdateClient backed by a ConfigMap that describes
// the currently-selected RAUC update channel + bundle.
//
// RAUC runs on the host OS. This client cannot call rauc directly; it
// writes the intended update state into a well-known ConfigMap
// "novanas-update-state" in the operator namespace. The host-side
// novanas-updater.service (installed by the OS wave) watches this
// ConfigMap via a kube API client and invokes `rauc install` locally.
//
// CurrentVersion / AvailableVersion read /etc/os-release-style values
// from the ConfigMap's status data populated by the host updater on
// completion.
package reconciler

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMapUpdateClient writes update intent to a ConfigMap and reads
// back the host updater's reported state.
type ConfigMapUpdateClient struct {
	Client    client.Client
	Namespace string
	// ConfigMapName defaults to "novanas-update-state".
	ConfigMapName string
}

// NewConfigMapUpdateClient constructs an UpdateClient with sensible defaults.
func NewConfigMapUpdateClient(c client.Client, namespace string) *ConfigMapUpdateClient {
	if namespace == "" {
		namespace = "novanas-system"
	}
	return &ConfigMapUpdateClient{
		Client:        c,
		Namespace:     namespace,
		ConfigMapName: "novanas-update-state",
	}
}

// CurrentVersion reads the host-reported current version.
func (u *ConfigMapUpdateClient) CurrentVersion(ctx context.Context) (string, error) {
	cm, err := u.fetch(ctx)
	if err != nil {
		return "", err
	}
	if cm == nil {
		return "", nil
	}
	return cm.Data["current_version"], nil
}

// AvailableVersion reads the host-reported latest version for a channel.
// Channels live under a per-channel key: "available_<channel>".
func (u *ConfigMapUpdateClient) AvailableVersion(ctx context.Context, channel string) (string, error) {
	cm, err := u.fetch(ctx)
	if err != nil {
		return "", err
	}
	if cm == nil {
		return "", nil
	}
	if channel == "" {
		channel = "stable"
	}
	return cm.Data["available_"+channel], nil
}

// Apply writes the requested version as the desired target. The host
// updater observes the change and begins installation. The caller
// records the returned job id in status.
func (u *ConfigMapUpdateClient) Apply(ctx context.Context, version string) (string, error) {
	if version == "" {
		return "", errors.New("update: empty version")
	}
	jobID := fmt.Sprintf("update-%s-%d", version, time.Now().Unix())
	cm, err := u.fetch(ctx)
	if err != nil {
		return "", err
	}
	if cm == nil {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      u.ConfigMapName,
				Namespace: u.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "novanas-operators",
				},
			},
			Data: map[string]string{},
		}
		cm.Data["desired_version"] = version
		cm.Data["job_id"] = jobID
		cm.Data["requested_at"] = time.Now().UTC().Format(time.RFC3339)
		if cerr := u.Client.Create(ctx, cm); cerr != nil {
			return "", fmt.Errorf("update: create configmap: %w", cerr)
		}
		return jobID, nil
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["desired_version"] = version
	cm.Data["job_id"] = jobID
	cm.Data["requested_at"] = time.Now().UTC().Format(time.RFC3339)
	if uerr := u.Client.Update(ctx, cm); uerr != nil {
		return "", fmt.Errorf("update: update configmap: %w", uerr)
	}
	return jobID, nil
}

func (u *ConfigMapUpdateClient) fetch(ctx context.Context) (*corev1.ConfigMap, error) {
	if u == nil || u.Client == nil {
		return nil, errors.New("update client: not configured")
	}
	cm := &corev1.ConfigMap{}
	err := u.Client.Get(ctx, types.NamespacedName{Name: u.ConfigMapName, Namespace: u.Namespace}, cm)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update: get configmap: %w", err)
	}
	return cm, nil
}

var _ UpdateClient = (*ConfigMapUpdateClient)(nil)
