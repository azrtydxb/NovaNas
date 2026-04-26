// Package controllers holds one reconciler per NovaNas CRD kind.
//
// The reconcilers share a common minimal-viable pattern implemented through
// helpers in packages/operators/internal/reconciler: finalizer management,
// condition tracking with observedGeneration, a small set of external-system
// adapter interfaces (KeycloakClient, StorageClient, CertificateIssuer), and
// event emission. Individual controllers embed reconciler.BaseReconciler
// and optionally accept concrete interface fields which default to no-op
// implementations when un-wired.
package controllers

const (
	// ConditionReady is the standard condition name controllers should use
	// to summarise overall resource health. Kept for backward compatibility
	// with callers that still reference controllers.ConditionReady; new code
	// should use reconciler.ConditionReady directly.
	ConditionReady = "Ready"

	// ConditionReconciling indicates an in-flight reconcile. Superseded by
	// reconciler.ConditionProgressing but kept for source-compatibility.
	ConditionReconciling = "Reconciling"

	// FinalizerPrefix mirrors reconciler.FinalizerPrefix. Duplicated as a
	// constant here so existing controllers can reference it without an
	// extra import; the reconciler package is the source of truth.
	FinalizerPrefix = "novanas.io/"

)
