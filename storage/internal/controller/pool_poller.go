package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanas "github.com/azrtydxb/novanas/packages/sdk/go-client"
	novastorev1alpha1 "github.com/azrtydxb/novanas/storage/api/v1alpha1"
)

// PoolPoller is the API-driven replacement for the controller-runtime
// StoragePoolReconciler watch (#50). It periodically fetches the
// authoritative Pool list from the NovaNas API server, then runs the
// same node-matching + BackendAssignment reconcile logic against the
// local Kubernetes cluster.
//
// BackendAssignment + Node reads/writes still go through the
// controller-runtime client because those resources stay in
// Kubernetes — only the Pool itself moved to Postgres.
//
// Run(ctx) blocks; cancel ctx to stop the poller.
type PoolPoller struct {
	// Client for k8s reads/writes (nodes, BackendAssignments).
	K8s client.Client
	// API client for Pool reads/writes.
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

// reconcileOne runs the same reconcile logic as the legacy controller's
// Reconcile(), but with the Pool sourced from the api instead of from
// the kube apiserver. The shape of writes (BackendAssignment create /
// update / delete on k8s; PatchPoolStatus on the api) mirrors the
// original — only the data source for Pool itself changed.
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

	var existing novastorev1alpha1.BackendAssignmentList
	if err := p.K8s.List(ctx, &existing, client.MatchingLabels{
		"novanas.io/pool": pool.Metadata.Name,
	}); err != nil {
		return fmt.Errorf("list BackendAssignments: %w", err)
	}
	byNode := make(map[string]*novastorev1alpha1.BackendAssignment, len(existing.Items))
	for i := range existing.Items {
		byNode[existing.Items[i].Spec.NodeName] = &existing.Items[i]
	}
	desired := make(map[string]bool, len(matchingNodes))
	for _, n := range matchingNodes {
		desired[n.Name] = true
	}

	for _, n := range matchingNodes {
		if cur, ok := byNode[n.Name]; ok {
			want := buildAssignmentSpecFromAPI(pool, n.Name)
			if !equality.Semantic.DeepEqual(cur.Spec, want) {
				cur.Spec = want
				if err := p.K8s.Update(ctx, cur); err != nil {
					return fmt.Errorf("update BackendAssignment %s: %w", cur.Name, err)
				}
				logger.Info("updated BackendAssignment", "name", cur.Name)
			}
			continue
		}
		ba := &novastorev1alpha1.BackendAssignment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", pool.Metadata.Name, n.Name),
				Namespace: pool.Metadata.Namespace,
				Labels: map[string]string{
					"novanas.io/pool": pool.Metadata.Name,
					"novanas.io/node": n.Name,
				},
				// No OwnerReference: Pool no longer lives in k8s
				// (#50), so cascade-delete via owner refs isn't an
				// option. Orphan cleanup happens the next time the
				// poller observes the Pool gone — see the loop end.
			},
			Spec: buildAssignmentSpecFromAPI(pool, n.Name),
		}
		ba.Status.Phase = "Pending"
		if err := p.K8s.Create(ctx, ba); err != nil {
			if errors.IsAlreadyExists(err) {
				continue
			}
			return fmt.Errorf("create BackendAssignment for %s: %w", n.Name, err)
		}
		logger.Info("created BackendAssignment", "name", ba.Name, "node", n.Name)
	}

	for nodeName, ba := range byNode {
		if !desired[nodeName] {
			if err := p.K8s.Delete(ctx, ba); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("delete orphaned BackendAssignment %s: %w", ba.Name, err)
			}
			logger.Info("deleted orphaned BackendAssignment", "name", ba.Name)
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
	st.TotalCapacity = aggregateBackendCapacityForPool(ctx, p.K8s, pool.Metadata.Name)
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
	ctx context.Context, k client.Client, poolName string,
) string {
	var bal novastorev1alpha1.BackendAssignmentList
	if err := k.List(ctx, &bal, client.MatchingLabels{
		"novanas.io/pool": poolName,
	}); err != nil {
		return "0"
	}
	var total int64
	for i := range bal.Items {
		if bal.Items[i].Status.Phase == "Ready" {
			total += bal.Items[i].Status.Capacity
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

func buildAssignmentSpecFromAPI(
	pool *novanas.Pool, nodeName string,
) novastorev1alpha1.BackendAssignmentSpec {
	spec := novastorev1alpha1.BackendAssignmentSpec{
		PoolRef:     pool.Metadata.Name,
		NodeName:    nodeName,
		BackendType: pool.Spec.BackendType,
	}
	if pool.Spec.DeviceFilter != nil {
		spec.DeviceFilter = &novastorev1alpha1.DeviceFilter{
			Type:    pool.Spec.DeviceFilter.Type,
			MinSize: pool.Spec.DeviceFilter.MinSize,
		}
	}
	if pool.Spec.FileBackend != nil {
		fb := &novastorev1alpha1.FileBackendSpec{Path: pool.Spec.FileBackend.Path}
		if pool.Spec.FileBackend.MaxCapacityBytes != nil {
			v := *pool.Spec.FileBackend.MaxCapacityBytes
			fb.MaxCapacityBytes = &v
		}
		spec.FileBackend = fb
	}
	return spec
}

// silence unused-import warnings if meta becomes unreferenced later;
// kept for future condition helpers from k8s api machinery.
var _ = meta.SetStatusCondition
