package policy

import (
	"context"
	"fmt"

	novanas "github.com/azrtydxb/novanas/packages/sdk/go-client"
	"github.com/azrtydxb/novanas/storage/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIPoolLookup implements PoolLookup against the NovaNas API server
// instead of the kube apiserver (#50). Used by the policy engine when
// the data plane has been switched off CRD watches.
type APIPoolLookup struct {
	api *novanas.Client
}

// NewAPIPoolLookup builds a PoolLookup wired to the api.
func NewAPIPoolLookup(api *novanas.Client) *APIPoolLookup {
	return &APIPoolLookup{api: api}
}

// ListPools fetches every Pool from /api/v1/pools and converts each
// to the in-cluster v1alpha1.StoragePool shape so the policy engine
// can consume it without changes.
func (l *APIPoolLookup) ListPools(ctx context.Context) ([]*v1alpha1.StoragePool, error) {
	pools, err := l.api.ListPools(ctx)
	if err != nil {
		return nil, fmt.Errorf("api list pools: %w", err)
	}
	out := make([]*v1alpha1.StoragePool, len(pools))
	for i := range pools {
		out[i] = toV1Alpha1(&pools[i])
	}
	return out, nil
}

// GetPool fetches a single Pool by name.
func (l *APIPoolLookup) GetPool(ctx context.Context, name string) (*v1alpha1.StoragePool, error) {
	p, err := l.api.GetPool(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("api get pool %s: %w", name, err)
	}
	return toV1Alpha1(p), nil
}

// toV1Alpha1 converts the SDK's wire-shaped Pool into the in-tree
// v1alpha1.StoragePool the policy engine expects. Only the fields the
// engine reads are populated — everything else stays zero.
func toV1Alpha1(p *novanas.Pool) *v1alpha1.StoragePool {
	out := &v1alpha1.StoragePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:        p.Metadata.Name,
			Namespace:   p.Metadata.Namespace,
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
		Spec: v1alpha1.StoragePoolSpec{
			BackendType: p.Spec.BackendType,
		},
	}
	if p.Spec.NodeSelector != nil {
		sel := &metav1.LabelSelector{MatchLabels: p.Spec.NodeSelector.MatchLabels}
		for _, r := range p.Spec.NodeSelector.MatchExpressions {
			sel.MatchExpressions = append(sel.MatchExpressions, metav1.LabelSelectorRequirement{
				Key:      r.Key,
				Operator: metav1.LabelSelectorOperator(r.Operator),
				Values:   r.Values,
			})
		}
		out.Spec.NodeSelector = sel
	}
	if p.Spec.DeviceFilter != nil {
		out.Spec.DeviceFilter = &v1alpha1.DeviceFilter{
			Type:    p.Spec.DeviceFilter.PreferredClass,
			MinSize: p.Spec.DeviceFilter.MinSize,
		}
	}
	if p.Spec.FileBackend != nil {
		fb := &v1alpha1.FileBackendSpec{Path: p.Spec.FileBackend.Path}
		if p.Spec.FileBackend.SizeBytes > 0 {
			size := p.Spec.FileBackend.SizeBytes
			fb.MaxCapacityBytes = &size
		}
		out.Spec.FileBackend = fb
	}
	out.Status.Phase = p.Status.Phase
	return out
}
