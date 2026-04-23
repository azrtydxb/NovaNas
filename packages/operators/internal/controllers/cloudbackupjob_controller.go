package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// CloudBackupJobReconciler drives a CloudBackupJob through its lifecycle.
//
// There are two modes:
//  1. One-shot (spec.cron unset): phases Pending → Running → Succeeded/Failed.
//  2. Scheduled (spec.cron set): the controller evaluates the cron
//     expression, starts a new run when NextRun ≤ now, and returns to
//     Scheduled after each completion with NextRun advanced.
//
// The controller delegates the heavy lifting to the injected
// StorageClient; the reconciler's job is state transitions, progress
// accounting, and the cron scheduler.
type CloudBackupJobReconciler struct {
	reconciler.BaseReconciler
	Storage  reconciler.StorageClient
	Recorder record.EventRecorder
	// Now is injected for deterministic cron tests. Defaults to time.Now.
	Now func() time.Time
}

// Reconcile implements the job state machine.
func (r *CloudBackupJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "CloudBackupJob", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.CloudBackupJob
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	sc := r.Storage
	if sc == nil {
		sc = reconciler.NoopStorageClient{}
	}
	now := r.Now
	if now == nil {
		now = time.Now
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("CloudBackupJob deleting")
		if err := sc.CancelBackup(ctx, string(obj.UID)); err != nil {
			logger.Error(err, "cancel backup failed")
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "cloud backup job cancelled")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCloudBackupJob); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCloudBackupJob); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation

	if err := validateCloudBackupJob(&obj.Spec); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.LastError = err.Error()
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonValidationFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// --- action-run-now annotation ----------------------------------
	if _, err := reconciler.HandleActionAnnotation(ctx, r.Client, &obj, "run-now",
		func(ctx context.Context, _ client.Object) error {
			logger.Info("action-run-now: restarting cloud backup job")
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonProvisioning, "cloud backup run-now requested")
			obj.Status.Phase = "Pending"
			return nil
		}); err != nil {
		logger.Error(err, "run-now handler failed")
	}

	// --- action-cancel annotation -----------------------------------
	if _, err := reconciler.HandleActionAnnotation(ctx, r.Client, &obj, "cancel",
		func(ctx context.Context, _ client.Object) error {
			logger.Info("action-cancel: cancelling cloud backup job")
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "cloud backup cancel requested")
			if cErr := sc.CancelBackup(ctx, string(obj.UID)); cErr != nil {
				return cErr
			}
			obj.Status.Phase = "Cancelled"
			return nil
		}); err != nil {
		logger.Error(err, "cancel handler failed")
	}
	if obj.Status.Phase == "Cancelled" {
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconciled, "cancelled by action annotation")
		if err := r.Client.Status().Update(ctx, &obj); err != nil && !apierrors.IsConflict(err) {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if obj.Spec.Suspended {
		obj.Status.Phase = "Suspended"
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconciled, "job suspended")
		_ = r.Client.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// --- cron scheduling ---------------------------------------------
	if obj.Spec.Cron != "" && obj.Status.Phase != "Running" {
		next, cErr := cronNextRun(obj.Spec.Cron, now().UTC())
		if cErr != nil {
			obj.Status.Phase = "Failed"
			obj.Status.LastError = cErr.Error()
			obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
				reconciler.ReasonValidationFailed, cErr.Error())
			_ = r.Client.Status().Update(ctx, &obj)
			result = "error"
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}
		nextT := metav1.NewTime(next)
		// If we were waiting on a NextRun that just elapsed, start now.
		if obj.Status.NextRun != nil && !obj.Status.NextRun.After(now()) {
			// kick off a run by forcing Pending
			obj.Status.Phase = "Pending"
		} else if obj.Status.Phase != "Pending" {
			obj.Status.Phase = "Scheduled"
			obj.Status.NextRun = &nextT
			obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
				reconciler.ReasonReconciled, "scheduled")
			if err := r.Client.Status().Update(ctx, &obj); err != nil && !apierrors.IsConflict(err) {
				result = "error"
				return ctrl.Result{}, err
			}
			wait := time.Until(next)
			if wait < 15*time.Second {
				wait = 15 * time.Second
			}
			return ctrl.Result{RequeueAfter: wait}, nil
		}
	}

	// --- start / poll a run ------------------------------------------
	var st reconciler.StorageOpStatus
	var err error
	if obj.Status.Phase == "" || obj.Status.Phase == "Pending" {
		nowTime := metav1.NewTime(now())
		obj.Status.LastRun = &nowTime
		st, err = sc.StartBackup(ctx, reconciler.BackupRequest{
			JobID:    string(obj.UID),
			VolumeID: volumeIDForSource(&obj.Spec.Source),
			Target:   obj.Spec.Target,
		})
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonProvisioning, "backup started")
	} else {
		st, err = sc.GetBackupStatus(ctx, string(obj.UID))
	}
	if err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.LastError = err.Error()
		obj.Status.FailureCount++
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconcileFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	obj.Status.BytesTransferred = st.BytesDone
	obj.Status.BytesTotal = st.BytesTotal
	obj.Status.ProgressPercent = st.Progress
	obj.Status.Phase = translateBackupPhase(st.Phase)
	switch obj.Status.Phase {
	case "Succeeded":
		ts := metav1.NewTime(now())
		obj.Status.LastSuccessfulRun = &ts
		obj.Status.LastError = ""
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconciled, st.Message)
		if obj.Spec.Cron != "" {
			if next, nErr := cronNextRun(obj.Spec.Cron, now().UTC()); nErr == nil {
				nt := metav1.NewTime(next)
				obj.Status.NextRun = &nt
				obj.Status.Phase = "Scheduled"
			}
		}
	case "Failed":
		obj.Status.FailureCount++
		obj.Status.LastError = st.Message
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconcileFailed, st.Message)
	default:
		obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconciling, st.Message)
	}

	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}

	switch obj.Status.Phase {
	case "Succeeded", "Failed":
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	case "Scheduled":
		if obj.Status.NextRun != nil {
			wait := time.Until(obj.Status.NextRun.Time)
			if wait < 15*time.Second {
				wait = 15 * time.Second
			}
			return ctrl.Result{RequeueAfter: wait}, nil
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	default:
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
}

// SetupWithManager registers the controller with the manager.
func (r *CloudBackupJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "CloudBackupJob"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "cloudbackupjob-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.CloudBackupJob{}).
		Named("CloudBackupJob").
		Complete(r)
}

func validateCloudBackupJob(spec *novanasv1alpha1.CloudBackupJobSpec) error {
	if spec.Target == "" {
		return fmt.Errorf("spec.target is required")
	}
	if spec.Source.Kind == "" || spec.Source.Name == "" {
		// Empty source is only allowed while the CR is being authored;
		// treat as validation warning not hard error so kubectl-apply
		// progress updates still land.
		return nil
	}
	switch spec.Source.Kind {
	case "BlockVolume", "Dataset", "Snapshot":
	default:
		return fmt.Errorf("spec.source.kind %q is invalid", spec.Source.Kind)
	}
	if spec.Cron != "" {
		if _, err := cronNextRun(spec.Cron, time.Now()); err != nil {
			return fmt.Errorf("spec.cron: %w", err)
		}
	}
	return nil
}

func translateBackupPhase(p string) string {
	switch p {
	case "Queued", "Pending":
		return "Pending"
	case "Running":
		return "Running"
	case "Completed", "Succeeded":
		return "Succeeded"
	case "Failed":
		return "Failed"
	default:
		return "Pending"
	}
}

func volumeIDForSource(s *novanasv1alpha1.VolumeSourceRef) string {
	if s == nil || s.Name == "" {
		return ""
	}
	return strings.ToLower(s.Kind) + "/" + s.Name
}

// ----- tiny cron parser ---------------------------------------------------

// cronNextRun returns the next fire time strictly after `from` for the
// supplied 5-field cron expression "minute hour day-of-month month
// day-of-week". Supported syntax per field:
//   *           any
//   N           fixed value
//   A,B,C       list
//   A-B         range
//   */N         step over the full range
//
// This is deliberately minimal — the scheduler only needs "when is the
// next run?" for short-term polling, never a long forecast.
func cronNextRun(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron needs 5 fields, got %d", len(fields))
	}
	min, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("minute: %w", err)
	}
	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("hour: %w", err)
	}
	dom, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("day-of-month: %w", err)
	}
	mon, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("month: %w", err)
	}
	dow, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("day-of-week: %w", err)
	}

	t := from.Add(time.Minute).Truncate(time.Minute)
	// Search forward at most 4 years (covers every leap cycle).
	deadline := t.Add(4 * 365 * 24 * time.Hour)
	for t.Before(deadline) {
		if !contains(mon, int(t.Month())) {
			// jump to first day of next month at 00:00
			y, m := t.Year(), t.Month()
			m++
			if m > 12 {
				m = 1
				y++
			}
			t = time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !contains(dom, t.Day()) || !contains(dow, int(t.Weekday())) {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Add(24 * time.Hour)
			continue
		}
		if !contains(hour, t.Hour()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !contains(min, t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("no cron match within 4 years")
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func parseCronField(f string, lo, hi int) ([]int, error) {
	out := make([]int, 0, hi-lo+1)
	seen := make(map[int]bool)
	for _, part := range strings.Split(f, ",") {
		step := 1
		base := part
		if i := strings.Index(part, "/"); i >= 0 {
			s, err := strconv.Atoi(part[i+1:])
			if err != nil || s <= 0 {
				return nil, fmt.Errorf("invalid step %q", part)
			}
			step = s
			base = part[:i]
		}
		var a, b int
		switch {
		case base == "*":
			a, b = lo, hi
		case strings.Contains(base, "-"):
			bounds := strings.SplitN(base, "-", 2)
			ai, err1 := strconv.Atoi(bounds[0])
			bi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid range %q", base)
			}
			a, b = ai, bi
		default:
			v, err := strconv.Atoi(base)
			if err != nil {
				return nil, fmt.Errorf("invalid value %q", base)
			}
			a, b = v, v
		}
		if a < lo || b > hi || a > b {
			return nil, fmt.Errorf("value out of range [%d,%d]: %s", lo, hi, part)
		}
		for v := a; v <= b; v += step {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no values")
	}
	return out, nil
}
