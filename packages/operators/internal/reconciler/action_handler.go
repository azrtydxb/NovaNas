// Package reconciler — action annotation helpers.
//
// E1 (API-Actions) introduced a small action-annotation vocabulary that
// operators consume to perform imperative operations (reset, renew,
// run-now, cancel, etc.) on top of the standard desired-state reconcile
// loop. The convention is:
//
//	novanas.io/action-<verb>: <rfc3339 timestamp>
//
// When an operator sees this annotation on a resource it performs the
// action and clears the annotation. On success it stamps a paired
// completion annotation so UIs can surface "last <verb> at <time>":
//
//	novanas.io/action-<verb>-completed: <rfc3339 timestamp>
//
// This file centralises the detect/clear/stamp logic so every
// reconciler handles annotations the same way.
package reconciler

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ActionAnnotationPrefix is the prefix used by E1 to trigger
	// operator actions. Verbs follow the dash: "action-reset",
	// "action-run-now", etc.
	ActionAnnotationPrefix = "novanas.io/action-"
	// ActionCompletedSuffix is appended to the verb to record a
	// successful completion timestamp.
	ActionCompletedSuffix = "-completed"
	// ActionFailedSuffix records the most recent failure timestamp
	// alongside the cleared trigger.
	ActionFailedSuffix = "-failed"
)

// ActionAnnotationName returns the full annotation key for a verb.
func ActionAnnotationName(verb string) string {
	return ActionAnnotationPrefix + verb
}

// ActionCompletedAnnotationName returns the completion-stamp key.
func ActionCompletedAnnotationName(verb string) string {
	return ActionAnnotationPrefix + verb + ActionCompletedSuffix
}

// ActionFailedAnnotationName returns the failure-stamp key.
func ActionFailedAnnotationName(verb string) string {
	return ActionAnnotationPrefix + verb + ActionFailedSuffix
}

// HandleActionAnnotation inspects obj for novanas.io/action-<verb>. If
// present it runs handler, then clears the trigger annotation and
// stamps the -completed (or -failed) counterpart. handled=true
// indicates the action was observed this reconcile (regardless of
// success), so the caller can decide to requeue.
//
// The resource is patched via JSON merge-patch so only the annotation
// map is written back — this avoids races with concurrent spec
// updates.
func HandleActionAnnotation(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	verb string,
	handler func(ctx context.Context, obj client.Object) error,
) (handled bool, err error) {
	ann := obj.GetAnnotations()
	trigger := ann[ActionAnnotationName(verb)]
	if trigger == "" {
		return false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	hErr := handler(ctx, obj)
	// Always attempt to clear the trigger — leaving a stale trigger
	// would retrigger on every reconcile.
	patched := obj.DeepCopyObject().(client.Object)
	pann := patched.GetAnnotations()
	if pann == nil {
		pann = map[string]string{}
	}
	delete(pann, ActionAnnotationName(verb))
	if hErr == nil {
		pann[ActionCompletedAnnotationName(verb)] = now
		delete(pann, ActionFailedAnnotationName(verb))
	} else {
		pann[ActionFailedAnnotationName(verb)] = now
	}
	patched.SetAnnotations(pann)
	// Use a Patch so we don't clobber unrelated changes. Swallow
	// not-found (resource was deleted mid-action) and conflict (next
	// reconcile will re-attempt) so callers see only terminal errors.
	if pErr := c.Patch(ctx, patched, client.MergeFrom(obj)); pErr != nil {
		if apierrors.IsNotFound(pErr) || apierrors.IsConflict(pErr) {
			return true, hErr
		}
		if hErr == nil {
			return true, pErr
		}
	}
	return true, hErr
}
