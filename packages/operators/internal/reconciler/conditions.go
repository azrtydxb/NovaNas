package reconciler

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard condition types NovaNas controllers emit. These mirror the
// Kubernetes-recommended vocabulary so that kubectl describe / dashboards
// surface them uniformly.
const (
	ConditionReady       = "Ready"
	ConditionProgressing = "Progressing"
	ConditionDegraded    = "Degraded"
	ConditionFailed      = "Failed"
)

// Standard reasons.
const (
	ReasonReconciling       = "Reconciling"
	ReasonReconciled        = "Reconciled"
	ReasonReconcileFailed   = "ReconcileFailed"
	ReasonDeleting          = "Deleting"
	ReasonAwaitingExternal  = "AwaitingExternalSystem"
	ReasonChildNotReady     = "ChildResourceNotReady"
	ReasonChildReady        = "ChildResourceReady"
	ReasonValidationFailed  = "ValidationFailed"
)

// SetCondition inserts or updates the matching condition on the slice,
// updating LastTransitionTime only when Status flips. generation is copied
// into ObservedGeneration. The resulting slice is returned.
func SetCondition(conds []metav1.Condition, c metav1.Condition, generation int64) []metav1.Condition {
	c.ObservedGeneration = generation
	now := metav1.NewTime(time.Now())
	for i := range conds {
		if conds[i].Type == c.Type {
			if conds[i].Status != c.Status {
				c.LastTransitionTime = now
			} else {
				c.LastTransitionTime = conds[i].LastTransitionTime
			}
			conds[i] = c
			return conds
		}
	}
	c.LastTransitionTime = now
	return append(conds, c)
}

// MarkReady sets Ready=True and clears Progressing/Failed.
func MarkReady(conds []metav1.Condition, generation int64, reason, msg string) []metav1.Condition {
	conds = SetCondition(conds, metav1.Condition{
		Type: ConditionReady, Status: metav1.ConditionTrue, Reason: reason, Message: msg,
	}, generation)
	conds = SetCondition(conds, metav1.Condition{
		Type: ConditionProgressing, Status: metav1.ConditionFalse, Reason: ReasonReconciled, Message: "reconcile complete",
	}, generation)
	conds = SetCondition(conds, metav1.Condition{
		Type: ConditionFailed, Status: metav1.ConditionFalse, Reason: ReasonReconciled, Message: "no error",
	}, generation)
	return conds
}

// MarkProgressing sets Progressing=True and Ready=False (if not already set).
func MarkProgressing(conds []metav1.Condition, generation int64, reason, msg string) []metav1.Condition {
	conds = SetCondition(conds, metav1.Condition{
		Type: ConditionProgressing, Status: metav1.ConditionTrue, Reason: reason, Message: msg,
	}, generation)
	conds = SetCondition(conds, metav1.Condition{
		Type: ConditionReady, Status: metav1.ConditionFalse, Reason: reason, Message: msg,
	}, generation)
	return conds
}

// MarkFailed sets Failed=True and Ready=False.
func MarkFailed(conds []metav1.Condition, generation int64, reason, msg string) []metav1.Condition {
	conds = SetCondition(conds, metav1.Condition{
		Type: ConditionFailed, Status: metav1.ConditionTrue, Reason: reason, Message: msg,
	}, generation)
	conds = SetCondition(conds, metav1.Condition{
		Type: ConditionReady, Status: metav1.ConditionFalse, Reason: reason, Message: msg,
	}, generation)
	return conds
}

// MarkDegraded sets Degraded=True but keeps Ready as-is; caller may choose
// to flip Ready themselves.
func MarkDegraded(conds []metav1.Condition, generation int64, reason, msg string) []metav1.Condition {
	return SetCondition(conds, metav1.Condition{
		Type: ConditionDegraded, Status: metav1.ConditionTrue, Reason: reason, Message: msg,
	}, generation)
}

// ClearDegraded explicitly flips Degraded off.
func ClearDegraded(conds []metav1.Condition, generation int64) []metav1.Condition {
	return SetCondition(conds, metav1.Condition{
		Type: ConditionDegraded, Status: metav1.ConditionFalse, Reason: ReasonReconciled, Message: "",
	}, generation)
}
