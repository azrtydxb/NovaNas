package reconciler

import (
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewRecorder returns an EventRecorder for the given controller name from
// the manager. Centralised here so controllers do not call
// mgr.GetEventRecorderFor directly -- when controller-runtime eventually
// renames or replaces that API (see upstream issue to migrate to the v1
// events recorder), the shim can be updated in one place.
//
// This helper abstracts the current GetEventRecorderFor API behind a name
// the project controls.
func NewRecorder(mgr ctrl.Manager, controllerName string) record.EventRecorder {
	if mgr == nil {
		return nil
	}
	// GetEventRecorderFor is deprecated in newer controller-runtime but the
	// replacement GetEventRecorder is not yet widely available. Pinning to
	// this shim so the migration is a one-line change when we bump deps.
	//lint:ignore SA1019 intentional — pending controller-runtime event API migration
	return mgr.GetEventRecorderFor(controllerName)
}

// Event reason constants used across controllers.
const (
	EventReasonCreated       = "Created"
	EventReasonUpdated       = "Updated"
	EventReasonDeleted       = "Deleted"
	EventReasonReady         = "Ready"
	EventReasonFailed        = "Failed"
	EventReasonProvisioning  = "Provisioning"
	EventReasonFinalizing    = "Finalizing"
	EventReasonReconcileErr  = "ReconcileError"
	EventReasonChildEnsured  = "ChildResourceEnsured"
	EventReasonExternalSync  = "ExternalSync"
)

// Emit publishes a normal event on rec for obj. If rec is nil the call is
// silently ignored -- this keeps controllers runnable without a recorder
// wired in for tests.
func Emit(rec record.EventRecorder, obj client.Object, reason, message string) {
	if rec == nil || obj == nil {
		return
	}
	rec.Event(obj, "Normal", reason, message)
}

// EmitWarning publishes a warning event on rec for obj.
func EmitWarning(rec record.EventRecorder, obj client.Object, reason, message string) {
	if rec == nil || obj == nil {
		return
	}
	rec.Event(obj, "Warning", reason, message)
}
