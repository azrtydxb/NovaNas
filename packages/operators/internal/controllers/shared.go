// Package controllers holds one reconciler per NovaNas CRD kind.
//
// Every controller here is currently a no-op: Reconcile returns without
// taking any action. Real logic lands in Wave 4+. The scaffolding exists so
// the manager binary builds, leader election works, and CRD kinds can be
// watched end-to-end in a local dev cluster before the hard work starts.
package controllers

const (
	// Finalizer is the common finalizer prefix for NovaNas-owned resources.
	// Concrete controllers append their kind:
	//
	//   novanas.io/storagepool
	//   novanas.io/dataset
	//   ...
	FinalizerPrefix = "novanas.io/"

	// ConditionReady is the standard condition name controllers should use
	// to summarise overall resource health.
	ConditionReady = "Ready"

	// ConditionReconciling indicates an in-flight reconcile.
	ConditionReconciling = "Reconciling"
)
