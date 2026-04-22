package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerGpuDevice = reconciler.FinalizerPrefix + "gpudevice"

// GpuDeviceReconciler is a status-only observer of a GPU device that
// also enforces passthrough assignment: when spec.assignedTo =
// {namespace,name} is set, the controller records the assignment on
// status and emits an event. Reassignment of a device already bound
// to a different Vm is refused — the caller must first clear
// spec.assignedTo.
type GpuDeviceReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile observes the GPU device.
func (r *GpuDeviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "GpuDevice", "key", req.NamespacedName)
	defer r.ObserveReconcile(start, "ok")

	var obj novanasv1alpha1.GpuDevice
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerGpuDevice); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerGpuDevice); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// --- assignedTo reconciliation ----------------------------------
	desiredNs, desiredName := readGpuAssignment(ctx, r.Client, obj.Name)
	currentNs := obj.Annotations[reconciler.ActionAnnotationPrefix+"assigned-namespace"]
	currentName := obj.Annotations[reconciler.ActionAnnotationPrefix+"assigned-name"]

	phase := "Observed"
	message := "gpu observed"
	if desiredName != "" {
		if currentName != "" && (currentName != desiredName || currentNs != desiredNs) {
			// Refuse reassignment — the caller must null out
			// spec.assignedTo first.
			logger.Info("gpu already assigned; refusing reassignment",
				"current", currentNs+"/"+currentName, "desired", desiredNs+"/"+desiredName)
			obj.Status.Phase = "Conflict"
			obj.Status.Conditions = reconciler.MarkFailed(
				obj.Status.Conditions, obj.Generation,
				"AlreadyAssigned",
				"gpu already assigned to "+currentNs+"/"+currentName+
					"; clear spec.assignedTo before reassigning",
			)
			if err := statusUpdate(ctx, r.Client, &obj); err != nil {
				return ctrl.Result{}, err
			}
			reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonFailed, "reassignment blocked")
			return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
		}
		// Record the assignment on the CR so subsequent reconciles
		// can detect reassignment attempts.
		patched := obj.DeepCopy()
		if patched.Annotations == nil {
			patched.Annotations = map[string]string{}
		}
		patched.Annotations[reconciler.ActionAnnotationPrefix+"assigned-namespace"] = desiredNs
		patched.Annotations[reconciler.ActionAnnotationPrefix+"assigned-name"] = desiredName
		if err := r.Client.Patch(ctx, patched, client.MergeFrom(&obj)); err != nil {
			logger.V(1).Info("assignment annotation write failed", "error", err.Error())
		} else {
			obj = *patched
		}
		phase = "Assigned"
		message = "assigned to " + desiredNs + "/" + desiredName
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonExternalSync, "gpu "+message)
	} else if currentName != "" {
		// spec.assignedTo cleared — release the device.
		logger.Info("gpu released", "previous", currentNs+"/"+currentName)
		patched := obj.DeepCopy()
		delete(patched.Annotations, reconciler.ActionAnnotationPrefix+"assigned-namespace")
		delete(patched.Annotations, reconciler.ActionAnnotationPrefix+"assigned-name")
		if err := r.Client.Patch(ctx, patched, client.MergeFrom(&obj)); err != nil {
			logger.V(1).Info("assignment clear failed", "error", err.Error())
		} else {
			obj = *patched
		}
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonExternalSync, "gpu released")
	}

	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, message)
	obj.Status.Phase = phase
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "GpuDevice "+phase)
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// readGpuAssignment returns spec.assignedTo.namespace and .name off
// the GpuDevice CR via unstructured. Empty strings when unset.
func readGpuAssignment(ctx context.Context, c client.Client, name string) (string, string) {
	gvk := schema.GroupVersionKind{Group: "novanas.io", Version: "v1alpha1", Kind: "GpuDevice"}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	if err := c.Get(ctx, types.NamespacedName{Name: name}, u); err != nil {
		return "", ""
	}
	ns, _, _ := unstructured.NestedString(u.Object, "spec", "assignedTo", "namespace")
	n, _, _ := unstructured.NestedString(u.Object, "spec", "assignedTo", "name")
	return ns, n
}

// SetupWithManager registers the controller with the manager.
func (r *GpuDeviceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "GpuDevice"
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.GpuDevice{}).
		Named("GpuDevice").
		Complete(r)
}
