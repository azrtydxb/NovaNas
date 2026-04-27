package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanas "github.com/azrtydxb/novanas/packages/sdk/go-client"
)

// PoolPoller is the API-driven replacement for the controller-runtime
// StoragePoolReconciler watch (#50). It periodically fetches the
// authoritative Pool list from the NovaNas API server, computes the
// desired BackendAssignment set, and reconciles it against the API.
//
// All NovaNas resources (Pool, BackendAssignment) live in Postgres now
// and are accessed via the API SDK. The Kubernetes client is still used
// solely for reading Node objects for label-selector matching.
//
// Run(ctx) blocks; cancel ctx to stop the poller.
type PoolPoller struct {
	// K8s client — used only to list Nodes. BackendAssignments and
	// Pools both flow through the API SDK now.
	K8s client.Client
	// API client for Pool and BackendAssignment reads/writes.
	API *novanas.Client
	// Poll interval. Defaults to 30s when zero.
	Interval time.Duration
}

func (p *PoolPoller) Run(ctx context.Context) error {
	logger := log.FromContext(ctx).WithValues("component", "pool-poller")
	interval := p.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	logger.Info("started", "interval", interval)
	// Tick once immediately so a fresh start doesn't have to wait
	// `interval` before the first reconcile.
	p.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			p.tick(ctx)
		}
	}
}

func (p *PoolPoller) tick(ctx context.Context) {
	logger := log.FromContext(ctx).WithValues("component", "pool-poller")
	pools, err := p.API.ListPools(ctx)
	if err != nil {
		logger.Error(err, "list pools from api failed")
		return
	}
	for i := range pools {
		if err := p.reconcileOne(ctx, &pools[i]); err != nil {
			logger.Error(err, "reconcile pool", "name", pools[i].Metadata.Name)
		}
	}
}

func (p *PoolPoller) reconcileOne(ctx context.Context, pool *novanas.Pool) error {
	selector := toLabelSelector(pool.Spec.NodeSelector)

	matchingNodes, err := p.matchingNodes(ctx, selector)
	if err != nil {
		return p.markFailed(ctx, pool, "NodeListError", err.Error())
	}

	if len(matchingNodes) == 0 {
		return p.markPending(ctx, pool, "NoMatchingNodes",
			"no nodes match the nodeSelector", 0)
	}

	if err := p.reconcileAssignments(ctx, pool, matchingNodes); err != nil {
		return fmt.Errorf("reconcile BackendAssignments: %w", err)
	}

	return p.markReady(ctx, pool, len(matchingNodes))
}

func (p *PoolPoller) matchingNodes(
	ctx context.Context, selector *metav1.LabelSelector,
) ([]corev1.Node, error) {
	var nl corev1.NodeList
	opts := []client.ListOption{}
	if selector != nil {
		sel, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector: %w", err)
		}
		opts = append(opts, client.MatchingLabelsSelector{Selector: sel})
	}
	if err := p.K8s.List(ctx, &nl, opts...); err != nil {
		return nil, err
	}
	return nl.Items, nil
}

func (p *PoolPoller) reconcileAssignments(
	ctx context.Context, pool *novanas.Pool, matchingNodes []corev1.Node,
) error {
	logger := log.FromContext(ctx)

	// All BAs in the system; filter by the pool label locally — the
	// API doesn't expose label selectors yet.
	all, err := p.API.ListBackendAssignments(ctx)
	if err != nil {
		return fmt.Errorf("list BackendAssignments: %w", err)
	}
	byNode := make(map[string]*novanas.BackendAssignment)
	for i := range all {
		ba := &all[i]
		if ba.Metadata.Labels["novanas.io/pool"] != pool.Metadata.Name {
			continue
		}
		byNode[ba.Spec.NodeName] = ba
	}
	desired := make(map[string]bool, len(matchingNodes))
	for _, n := range matchingNodes {
		desired[n.Name] = true
	}

	for _, n := range matchingNodes {
		want := buildAssignmentSpecFromAPI(pool, n.Name)
		if cur, ok := byNode[n.Name]; ok {
			if !reflect.DeepEqual(cur.Spec, want) {
				if err := p.API.PatchBackendAssignmentSpec(ctx, cur.Metadata.Name, want); err != nil {
					return fmt.Errorf("update BackendAssignment %s: %w", cur.Metadata.Name, err)
				}
				logger.Info("updated BackendAssignment", "name", cur.Metadata.Name)
			}
			continue
		}
		ba := &novanas.BackendAssignment{
			APIVersion: "novanas.io/v1alpha1",
			Kind:       "BackendAssignment",
			Metadata: novanas.ObjectMeta{
				Name: fmt.Sprintf("%s-%s", pool.Metadata.Name, n.Name),
				Labels: map[string]string{
					"novanas.io/pool": pool.Metadata.Name,
					"novanas.io/node": n.Name,
				},
				// No OwnerReference: Pool no longer lives in k8s, and
				// the BackendAssignment is now also a Postgres row.
				// Orphan cleanup happens further down via DELETE.
			},
			Spec: want,
		}
		if _, err := p.API.CreateBackendAssignment(ctx, ba); err != nil {
			// 409 (already exists) is fine — another tick beat us to it.
			if apiErr, ok := err.(*novanas.APIError); ok && apiErr.Status == 409 {
				continue
			}
			return fmt.Errorf("create BackendAssignment for %s: %w", n.Name, err)
		}
		logger.Info("created BackendAssignment", "name", ba.Metadata.Name, "node", n.Name)
	}

	for nodeName, ba := range byNode {
		if !desired[nodeName] {
			if err := p.API.DeleteBackendAssignment(ctx, ba.Metadata.Name); err != nil {
				return fmt.Errorf("delete orphaned BackendAssignment %s: %w", ba.Metadata.Name, err)
			}
			logger.Info("deleted orphaned BackendAssignment", "name", ba.Metadata.Name)
		}
	}
	return nil
}

func (p *PoolPoller) markFailed(ctx context.Context, pool *novanas.Pool, reason, msg string) error {
	st := pool.Status
	st.Phase = "Pending"
	setCondition(&st.Conditions, "Ready", "False", reason, msg)
	return p.API.PatchPoolStatus(ctx, pool.Metadata.Name, st)
}

func (p *PoolPoller) markPending(
	ctx context.Context, pool *novanas.Pool, reason, msg string, nodeCount int,
) error {
	st := pool.Status
	st.Phase = "Pending"
	st.NodeCount = nodeCount
	st.TotalCapacity = "0"
	setCondition(&st.Conditions, "Ready", "False", reason, msg)
	return p.API.PatchPoolStatus(ctx, pool.Metadata.Name, st)
}

func (p *PoolPoller) markReady(ctx context.Context, pool *novanas.Pool, nodeCount int) error {
	st := pool.Status
	st.Phase = "Ready"
	st.NodeCount = nodeCount
	st.TotalCapacity = aggregateBackendCapacityForPool(ctx, p.API, pool.Metadata.Name)
	setCondition(&st.Conditions, "Ready", "True", "PoolReady",
		fmt.Sprintf("pool is ready with %d node(s)", nodeCount))
	return p.API.PatchPoolStatus(ctx, pool.Metadata.Name, st)
}

// setCondition appends/updates a single named condition by Type.
func setCondition(conds *[]novanas.Condition, ctype, status, reason, msg string) {
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range *conds {
		if (*conds)[i].Type == ctype {
			(*conds)[i] = novanas.Condition{
				Type: ctype, Status: status, Reason: reason, Message: msg,
				LastTransitionTime: now,
			}
			return
		}
	}
	*conds = append(*conds, novanas.Condition{
		Type: ctype, Status: status, Reason: reason, Message: msg,
		LastTransitionTime: now,
	})
}

func aggregateBackendCapacityForPool(
	ctx context.Context, api *novanas.Client, poolName string,
) string {
	all, err := api.ListBackendAssignments(ctx)
	if err != nil {
		return "0"
	}
	var total int64
	for i := range all {
		ba := &all[i]
		if ba.Metadata.Labels["novanas.io/pool"] != poolName {
			continue
		}
		if ba.Status.Phase == "Ready" {
			total += ba.Status.Capacity
		}
	}
	return formatCapacity(total)
}

// toLabelSelector converts the SDK's LabelSelector envelope back to
// the metav1 form expected by client.MatchingLabelsSelector.
func toLabelSelector(in *novanas.LabelSelector) *metav1.LabelSelector {
	if in == nil {
		return nil
	}
	out := &metav1.LabelSelector{MatchLabels: in.MatchLabels}
	for _, r := range in.MatchExpressions {
		out.MatchExpressions = append(out.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      r.Key,
			Operator: metav1.LabelSelectorOperator(r.Operator),
			Values:   r.Values,
		})
	}
	return out
}

// buildAssignmentSpecFromAPI translates a Pool's spec into the
// BackendAssignment.Spec a node should provision. Pool.Spec and the
// BackendAssignment wire shape now use identical field names
// (preferredClass / minSize / sizeBytes), so this is a pure copy with
// a single default: empty BackendType falls back to "raw" — the only
// dev-friendly option that doesn't need cluster-side prep.
func buildAssignmentSpecFromAPI(
	pool *novanas.Pool, nodeName string,
) novanas.BackendAssignmentSpec {
	backendType := pool.Spec.BackendType
	if backendType == "" {
		backendType = "raw"
	}
	spec := novanas.BackendAssignmentSpec{
		PoolRef:     pool.Metadata.Name,
		NodeName:    nodeName,
		BackendType: backendType,
	}
	if pool.Spec.DeviceFilter != nil {
		spec.DeviceFilter = &novanas.APIDeviceFilter{
			PreferredClass: pool.Spec.DeviceFilter.PreferredClass,
			MinSize:        pool.Spec.DeviceFilter.MinSize,
			MaxSize:        pool.Spec.DeviceFilter.MaxSize,
		}
	}
	if pool.Spec.FileBackend != nil {
		spec.FileBackend = &novanas.APIFileBackendSpec{
			Path:      pool.Spec.FileBackend.Path,
			SizeBytes: pool.Spec.FileBackend.SizeBytes,
		}
	}
	return spec
}
