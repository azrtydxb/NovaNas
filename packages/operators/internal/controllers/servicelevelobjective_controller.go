package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

const finalizerServiceLevelObjective = reconciler.FinalizerPrefix + "servicelevelobjective"

// PromQLClient is the minimal surface this controller calls on Prometheus.
// Tests inject a stub.
type PromQLClient interface {
	// Instant executes an instant query against /api/v1/query and returns
	// the first scalar/vector sample's value.
	Instant(ctx context.Context, baseURL, q string) (float64, error)
}

// ServiceLevelObjectiveReconciler projects SLOs into a PrometheusRule
// (burn-rate recording + alerting rules) and, via PromQLClient, polls
// the current SLI and maintains .status (CurrentSLI, BurnRate,
// ErrorBudgetRemainingSeconds).
type ServiceLevelObjectiveReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
	// Prom defaults to an http.Client-backed implementation when nil.
	Prom PromQLClient
	// Now is injected for deterministic tests.
	Now func() time.Time
}

func (r *ServiceLevelObjectiveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "ServiceLevelObjective", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.ServiceLevelObjective
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "SLO removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerServiceLevelObjective); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerServiceLevelObjective); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation,
		reconciler.ReasonReconciling, "projecting SLO rules")
	if obj.Status.Phase == "" {
		obj.Status.Phase = "Pending"
	}

	if err := validateSLO(&obj.Spec); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.LastError = err.Error()
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonValidationFailed, err.Error())
		_ = statusUpdate(ctx, r.Client, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// --- project PrometheusRule --------------------------------------
	ruleName := childName(obj.Name, "slo")
	obj.Status.RenderedRuleName = ruleName
	gvk := schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PrometheusRule"}
	err := ensureUnstructured(ctx, r.Client, gvk, "novanas-system", ruleName, func(u *unstructuredType) {
		u.SetLabels(map[string]string{"novanas.io/owner": obj.Name, "novanas.io/kind": "SLO"})
		setSpec(u, renderSLOPrometheusRule(&obj))
	})
	status := "Active"
	switch err {
	case nil:
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconciled, "SLO rules projected")
	case errKindMissing:
		logger.V(1).Info("PrometheusRule CRD absent; status-only mode")
		status = "Pending"
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonAwaitingExternal, "prometheus-operator absent; status-only")
	default:
		obj.Status.Phase = "Failed"
		obj.Status.LastError = err.Error()
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			"ProjectionFailed", err.Error())
		_ = statusUpdate(ctx, r.Client, &obj)
		result = "error"
		return ctrl.Result{}, err
	}

	// --- evaluate current SLI + burn rate ----------------------------
	// Best-effort: failure to reach Prometheus doesn't flip phase to
	// Failed; it just leaves LastError set and keeps the previous SLI.
	prom := r.Prom
	if prom == nil {
		prom = &httpPromClient{c: &http.Client{Timeout: 5 * time.Second}}
	}
	base := obj.Spec.PrometheusURL
	if base == "" {
		base = "http://prometheus.novanas-system:9090"
	}
	nowFn := r.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	sli, evalErr := evalSLI(ctx, prom, base, obj.Spec.Indicator)
	evalTS := metav1.NewTime(nowFn())
	obj.Status.LastEvaluation = &evalTS
	if evalErr != nil {
		obj.Status.LastError = evalErr.Error()
		reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonExternalSync, evalErr.Error())
		// We still persist partial status + a soft phase of Pending.
		if status == "Active" {
			status = "Pending"
		}
	} else {
		obj.Status.LastError = ""
		target := atof(obj.Spec.Target)
		obj.Status.CurrentSLI = fmt.Sprintf("%.4f", sli*100)

		windowSecs := durationSecs(obj.Spec.Window)
		if windowSecs > 0 && target > 0 && target < 100 {
			errorBudget := (100 - target) / 100 // fraction of window that may fail
			observedErr := 1 - sli               // fraction failing
			var burn float64
			if errorBudget > 0 {
				burn = observedErr / errorBudget
			}
			obj.Status.BurnRate = fmt.Sprintf("%.4f", burn)
			remainingFrac := errorBudget - observedErr
			if remainingFrac < 0 {
				remainingFrac = 0
			}
			obj.Status.ErrorBudgetRemainingSeconds = int64(remainingFrac * windowSecs)
			if errorBudget > 0 {
				obj.Status.ErrorBudgetRemainingPercent = fmt.Sprintf("%.2f", (remainingFrac/errorBudget)*100)
			}
			switch {
			case burn >= 14.4:
				status = "Breached"
			case burn >= 1.0:
				status = "AtRisk"
			default:
				if status == "Active" || status == "Pending" {
					status = "Active"
				}
			}
		}
	}
	obj.Status.Phase = status

	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		result = "error"
		return ctrl.Result{}, err
	}

	// Requeue on EvalIntervalSeconds or default.
	interval := 60 * time.Second
	if obj.Spec.EvalIntervalSeconds > 0 {
		interval = time.Duration(obj.Spec.EvalIntervalSeconds) * time.Second
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *ServiceLevelObjectiveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "ServiceLevelObjective"
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "slo-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.ServiceLevelObjective{}).
		Named("ServiceLevelObjective").
		Complete(r)
}

func validateSLO(spec *novanasv1alpha1.ServiceLevelObjectiveSpec) error {
	if spec.Window == "" {
		return fmt.Errorf("spec.window is required")
	}
	if spec.Indicator.GoodQuery == "" || spec.Indicator.TotalQuery == "" {
		return fmt.Errorf("spec.indicator.goodQuery and totalQuery are required")
	}
	t := atof(spec.Target)
	if t <= 0 || t > 100 {
		return fmt.Errorf("spec.target must be in (0,100]")
	}
	return nil
}

func renderSLOPrometheusRule(obj *novanasv1alpha1.ServiceLevelObjective) map[string]interface{} {
	sliRatio := fmt.Sprintf("(%s) / (%s)", obj.Spec.Indicator.GoodQuery, obj.Spec.Indicator.TotalQuery)
	errRatio := fmt.Sprintf("1 - (%s)", sliRatio)
	target := atof(obj.Spec.Target) / 100
	errBudget := 1 - target
	rules := []interface{}{
		map[string]interface{}{
			"record": obj.Name + ":sli",
			"expr":   sliRatio,
			"labels": map[string]interface{}{"novanas_slo": obj.Name},
		},
		map[string]interface{}{
			"record": obj.Name + ":error_ratio",
			"expr":   errRatio,
			"labels": map[string]interface{}{"novanas_slo": obj.Name},
		},
	}
	for _, a := range obj.Spec.BurnRateAlerts {
		thr := a.Threshold
		if thr == "" {
			thr = "14.4"
		}
		sev := a.Severity
		if sev == "" {
			sev = "critical"
		}
		shortW := a.ShortWindow
		if shortW == "" {
			shortW = "5m"
		}
		longW := a.LongWindow
		if longW == "" {
			longW = "1h"
		}
		expr := fmt.Sprintf(
			"(%s) and (%s)",
			fmt.Sprintf("( (%s)[%s:] / %f ) > %s", errRatio, shortW, errBudget, thr),
			fmt.Sprintf("( (%s)[%s:] / %f ) > %s", errRatio, longW, errBudget, thr),
		)
		rules = append(rules, map[string]interface{}{
			"alert": obj.Name + "_BurnRate_" + shortW + "_" + longW,
			"expr":  expr,
			"labels": map[string]interface{}{
				"severity":       sev,
				"novanas_slo":    obj.Name,
				"novanas_window": shortW + "/" + longW,
			},
			"annotations": map[string]interface{}{
				"summary": fmt.Sprintf("SLO %s burn-rate exceeds %s over %s/%s", obj.Name, thr, shortW, longW),
			},
		})
	}
	return map[string]interface{}{
		"groups": []interface{}{
			map[string]interface{}{
				"name":  obj.Name + "-slo",
				"rules": rules,
			},
		},
	}
}

// evalSLI issues two instant queries and returns good/total.
func evalSLI(ctx context.Context, p PromQLClient, base string, ind novanasv1alpha1.SLOIndicator) (float64, error) {
	good, err := p.Instant(ctx, base, ind.GoodQuery)
	if err != nil {
		return 0, fmt.Errorf("good query: %w", err)
	}
	total, err := p.Instant(ctx, base, ind.TotalQuery)
	if err != nil {
		return 0, fmt.Errorf("total query: %w", err)
	}
	if total <= 0 {
		return 1, nil
	}
	return good / total, nil
}

func durationSecs(s string) float64 {
	if s == "" {
		return 0
	}
	// Support "Nd", "Nh", "Nm", "Ns" and fall back to time.ParseDuration.
	last := s[len(s)-1]
	if last == 'd' {
		if n, err := strconv.Atoi(s[:len(s)-1]); err == nil {
			return float64(n) * 86400
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d.Seconds()
	}
	return 0
}

func atof(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// ----- httpPromClient ----------------------------------------------------

type httpPromClient struct{ c *http.Client }

// prom response shape, partial.
type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

func (p *httpPromClient) Instant(ctx context.Context, base, q string) (float64, error) {
	u, err := url.Parse(base)
	if err != nil {
		return 0, err
	}
	u.Path = "/api/v1/query"
	qv := url.Values{}
	qv.Set("query", q)
	u.RawQuery = qv.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	resp, err := p.c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prom HTTP %d", resp.StatusCode)
	}
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var pr promResponse
	if err := json.Unmarshal(buf, &pr); err != nil {
		return 0, err
	}
	if pr.Status != "success" {
		return 0, fmt.Errorf("prom error: %s", pr.Error)
	}
	switch pr.Data.ResultType {
	case "scalar":
		var arr []interface{}
		if err := json.Unmarshal(pr.Data.Result, &arr); err != nil {
			return 0, err
		}
		if len(arr) < 2 {
			return 0, fmt.Errorf("malformed scalar")
		}
		return toFloat(arr[1])
	case "vector":
		var vec []struct {
			Value []interface{} `json:"value"`
		}
		if err := json.Unmarshal(pr.Data.Result, &vec); err != nil {
			return 0, err
		}
		if len(vec) == 0 {
			return 0, nil
		}
		if len(vec[0].Value) < 2 {
			return 0, fmt.Errorf("malformed vector sample")
		}
		return toFloat(vec[0].Value[1])
	default:
		return 0, fmt.Errorf("unexpected resultType %q", pr.Data.ResultType)
	}
}

func toFloat(v interface{}) (float64, error) {
	switch x := v.(type) {
	case string:
		return strconv.ParseFloat(x, 64)
	case float64:
		return x, nil
	default:
		return 0, fmt.Errorf("not numeric: %T", v)
	}
}
