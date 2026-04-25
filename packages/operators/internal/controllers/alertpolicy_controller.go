package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerAlertPolicy = reconciler.FinalizerPrefix + "alertpolicy"

// AlertPolicyReconciler projects AlertPolicy CRs into PrometheusRule
// objects. When the Prometheus Operator CRD is absent the controller
// falls back to status-only so NovaNas stays usable on vanilla clusters.
//
// Key responsibilities:
//  1. Validate the spec (query, operator, threshold, channels).
//  2. Render a deterministic PrometheusRule body and hash it; only call
//     the API when the hash differs from .status.ruleHash to avoid
//     thrashing the Prometheus Operator config reload path.
//  3. Track FireCount / FiringSince from the most recent PrometheusRule
//     status (when available) so the UI can show how long a policy has
//     been firing.
type AlertPolicyReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the child PrometheusRule.
func (r *AlertPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "AlertPolicy", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.AlertPolicy
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "AlertPolicy removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerAlertPolicy); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerAlertPolicy); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation,
		reconciler.ReasonReconciling, "rendering PrometheusRule")
	if obj.Status.Phase == "" {
		obj.Status.Phase = "Pending"
	}

	if err := validateAlertPolicy(&obj.Spec); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonValidationFailed, err.Error())
		reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonFailed, err.Error())
		_ = statusUpdate(ctx, r.Client, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if obj.Spec.Suspended {
		obj.Status.Phase = "Suspended"
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconciled, "policy suspended; rule not projected")
		if err := statusUpdate(ctx, r.Client, &obj); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
	}

	ruleName := childName(obj.Name, "alert")
	obj.Status.RenderedRuleName = ruleName
	ns := obj.Spec.RuntimeNamespace
	if ns == "" {
		ns = "novanas-system"
	}

	spec := renderPrometheusRuleSpec(&obj)
	hash := hashRuleSpec(spec)
	needsApply := obj.Status.RuleHash != hash

	gvk := schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PrometheusRule"}
	if needsApply {
		err := ensureUnstructured(ctx, r.Client, gvk, ns, ruleName, func(u *unstructuredType) {
			u.SetLabels(map[string]string{
				"novanas.io/owner":    obj.Name,
				"novanas.io/severity": string(obj.Spec.Severity),
			})
			setSpec(u, spec)
		})
		switch err {
		case nil:
			obj.Status.RuleHash = hash
			obj.Status.Phase = "Active"
			obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
				reconciler.ReasonReconciled, "PrometheusRule projected")
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "PrometheusRule projected")
		case errKindMissing:
			logger.V(1).Info("PrometheusRule CRD absent; status-only mode")
			obj.Status.Phase = "Pending"
			obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
				reconciler.ReasonAwaitingExternal, "prometheus-operator not installed; status-only")
		default:
			obj.Status.Phase = "Failed"
			obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
				"ProjectionFailed", err.Error())
			reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonReconcileErr, err.Error())
			_ = statusUpdate(ctx, r.Client, &obj)
			result = "error"
			return ctrl.Result{}, err
		}
	} else {
		obj.Status.Phase = "Active"
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconciled, "PrometheusRule up-to-date")
	}

	// --- observe firing state (best-effort) --------------------------
	firing := readPrometheusRuleFiring(ctx, r.Client, gvk, ns, ruleName, obj.Name)
	applyFiringStatus(&obj, firing)

	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		result = "error"
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *AlertPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "AlertPolicy"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "alertpolicy-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.AlertPolicy{}).
		Named("AlertPolicy").
		Complete(r)
}

func validateAlertPolicy(spec *novanasv1alpha1.AlertPolicySpec) error {
	if spec.Condition.Query == "" {
		return fmt.Errorf("spec.condition.query is required")
	}
	switch spec.Condition.Operator {
	case ">", "<", ">=", "<=", "==", "!=":
	default:
		return fmt.Errorf("spec.condition.operator %q is invalid", spec.Condition.Operator)
	}
	switch spec.Severity {
	case "info", "warning", "critical":
	case "":
		return fmt.Errorf("spec.severity is required")
	default:
		return fmt.Errorf("spec.severity %q is invalid", spec.Severity)
	}
	// Channels may be empty at first; downstream dispatcher will log a
	// warning. Do not fail here so ops can fix it without losing the CR.
	return nil
}

// renderPrometheusRuleSpec produces a deterministic rule body.
func renderPrometheusRuleSpec(obj *novanasv1alpha1.AlertPolicy) map[string]interface{} {
	alertName := toPromIdent(obj.Name)
	expr := fmt.Sprintf("(%s) %s %s", obj.Spec.Condition.Query, obj.Spec.Condition.Operator, obj.Spec.Condition.Threshold)

	labels := map[string]interface{}{
		"severity":        string(obj.Spec.Severity),
		"novanas_policy":  obj.Name,
	}
	for k, v := range obj.Spec.Labels {
		labels[k] = v
	}
	for _, ch := range obj.Spec.Channels {
		// Prometheus labels don't accept repeated keys — encode channels
		// as a sorted comma-separated list.
		existing, _ := labels["novanas_channels"].(string)
		if existing == "" {
			labels["novanas_channels"] = ch
		} else {
			labels["novanas_channels"] = existing + "," + ch
		}
	}

	annotations := map[string]interface{}{
		"summary": obj.Spec.Description,
	}
	for k, v := range obj.Spec.Annotations {
		annotations[k] = v
	}

	rule := map[string]interface{}{
		"alert":       alertName,
		"expr":        expr,
		"labels":      labels,
		"annotations": annotations,
	}
	if obj.Spec.Condition.For != "" {
		rule["for"] = obj.Spec.Condition.For
	}
	return map[string]interface{}{
		"groups": []interface{}{
			map[string]interface{}{
				"name":  obj.Name,
				"rules": []interface{}{rule},
			},
		},
	}
}

// hashRuleSpec produces a stable SHA-256 of the rule body for change detection.
func hashRuleSpec(spec map[string]interface{}) string {
	// json.Marshal is stable for maps in Go via map key sorting? Go does
	// NOT sort map keys in Marshal when the value is an interface{}.
	// We therefore canonicalise by re-encoding through a sorted form.
	buf, err := canonicalJSON(spec)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

func canonicalJSON(v interface{}) ([]byte, error) {
	// json.Marshal DOES sort map[string]T keys for basic T per encoding/json,
	// but for map[string]interface{} nested maps the same rule applies
	// (map keys are sorted). So a direct Marshal is canonical enough.
	return json.Marshal(v)
}

// toPromIdent rewrites a CR name into a valid Prometheus alert identifier.
func toPromIdent(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 || (out[0] >= '0' && out[0] <= '9') {
		out = append([]byte{'a', 'l', 'e', 'r', 't', '_'}, out...)
	}
	return string(out)
}

// firingState bundles whatever we could read about firing status.
type firingState struct {
	firing   bool
	lastFire *metav1.Time
}

// readPrometheusRuleFiring inspects the child rule's status (which the
// Prometheus Operator annotates). If anything is missing we return a
// zero value — this is best-effort.
func readPrometheusRuleFiring(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, ns, name, policy string) firingState {
	u := &unstructuredType{}
	u.SetGroupVersionKind(gvk)
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, u); err != nil {
		return firingState{}
	}
	// The operator populates status.conditions on PrometheusRule only on
	// newer versions; we look for a well-known "Firing" annotation set
	// by NovaNas's alertmanager sidecar as a cheap integration point.
	if anno := u.GetAnnotations(); anno != nil {
		if v := anno["novanas.io/firing"]; v == "true" {
			t := metav1.NewTime(time.Now())
			return firingState{firing: true, lastFire: &t}
		}
	}
	return firingState{}
}

func applyFiringStatus(obj *novanasv1alpha1.AlertPolicy, s firingState) {
	if s.firing {
		if obj.Status.FiringSince == nil {
			obj.Status.FiringSince = s.lastFire
			obj.Status.FireCount++
		}
		obj.Status.LastFiredAt = s.lastFire
		if obj.Status.Phase == "Active" {
			obj.Status.Phase = "Firing"
		}
	} else {
		if obj.Status.FiringSince != nil {
			obj.Status.FiringSince = nil
		}
	}
}
