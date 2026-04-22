package reconciler

import (
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
