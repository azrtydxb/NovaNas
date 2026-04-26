package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanas "github.com/azrtydxb/novanas/packages/sdk/go-client"
	novastorev1alpha1 "github.com/azrtydxb/novanas/storage/api/v1alpha1"
)

const (
	csiDriverName = "novanas.csi.novanas.io"
)

// BlockVolumeReconciler reconciles BlockVolume objects.
//
// API field (#50): when non-nil, the StoragePool that a BlockVolume
// references is fetched via HTTP from the NovaNas API server instead
// of the k8s apiserver. The BlockVolume itself stays a CRD (grey-set
// per #50 acceptance), so the controller-runtime watch on it is
// preserved. Set in cmd/controller/main.go when the env opts in.
type BlockVolumeReconciler struct {
	client.Client
	API *novanas.Client
}

// +kubebuilder:rbac:groups=novanas.io,resources=blockvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=novanas.io,resources=blockvolumes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=novanas.io,resources=blockvolumes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles a single reconciliation request for a BlockVolume.
func (r *BlockVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var volume novastorev1alpha1.BlockVolume
	if err := r.Get(ctx, req.NamespacedName, &volume); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("reconciling BlockVolume", "name", req.Name, "namespace", req.Namespace)

	// Look up the referenced StoragePool. Source switches based on
	// whether the api client is wired (#50): api when present, k8s
	// otherwise (legacy CRD path).
	poolPhase, found, lookupErr := r.lookupPoolPhase(ctx, volume.Spec.Pool)
	if lookupErr != nil {
		return ctrl.Result{}, lookupErr
	}
	if !found {
		meta.SetStatusCondition(&volume.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "PoolNotFound",
			Message:            fmt.Sprintf("StoragePool %q not found", volume.Spec.Pool),
			ObservedGeneration: volume.Generation,
		})
		volume.Status.Phase = "Pending"
		if statusErr := r.Status().Update(ctx, &volume); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check pool readiness.
	if poolPhase != "Ready" {
		meta.SetStatusCondition(&volume.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "PoolNotReady",
			Message:            fmt.Sprintf("StoragePool %q is not ready (phase: %s)", volume.Spec.Pool, poolPhase),
			ObservedGeneration: volume.Generation,
		})
		volume.Status.Phase = "Pending"
		if statusErr := r.Status().Update(ctx, &volume); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Set status fields from spec.
	volume.Status.Pool = volume.Spec.Pool
	volume.Status.AccessMode = volume.Spec.AccessMode

	// Create or update the PersistentVolume.
	pvName := fmt.Sprintf("novanas-%s-%s", volume.Namespace, volume.Name)
	pv := &corev1.PersistentVolume{}
	pv.Name = pvName

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, pv, func() error {
		// Parse size.
		qty, parseErr := resource.ParseQuantity(volume.Spec.Size)
		if parseErr != nil {
			return fmt.Errorf("invalid volume size %q: %w", volume.Spec.Size, parseErr)
		}

		// Map access mode.
		var accessMode corev1.PersistentVolumeAccessMode
		switch volume.Spec.AccessMode {
		case "ReadWriteOnce":
			accessMode = corev1.ReadWriteOnce
		case "ReadOnlyMany":
			accessMode = corev1.ReadOnlyMany
		default:
			accessMode = corev1.ReadWriteOnce
		}

		pv.Spec = corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: qty,
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       csiDriverName,
					VolumeHandle: fmt.Sprintf("%s/%s", volume.Namespace, volume.Name),
					VolumeAttributes: map[string]string{
						"pool":       volume.Spec.Pool,
						"accessMode": volume.Spec.AccessMode,
					},
				},
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
		}

		// Set labels to track ownership (PVs are cluster-scoped so we cannot
		// set a namespaced owner reference).
		if pv.Labels == nil {
			pv.Labels = map[string]string{}
		}
		pv.Labels["novanas.io/blockvolume"] = volume.Name
		pv.Labels["novanas.io/namespace"] = volume.Namespace

		return nil
	})
	if err != nil {
		meta.SetStatusCondition(&volume.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "PVCreateError",
			Message:            fmt.Sprintf("failed to create PersistentVolume: %v", err),
			ObservedGeneration: volume.Generation,
		})
		volume.Status.Phase = "Pending"
		if statusErr := r.Status().Update(ctx, &volume); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	logger.Info("PersistentVolume reconciled", "name", pvName, "operation", result)

	// Volume is bound.
	volume.Status.Phase = "Bound"
	meta.SetStatusCondition(&volume.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "VolumeBound",
		Message:            fmt.Sprintf("PersistentVolume %s created and bound", pvName),
		ObservedGeneration: volume.Generation,
	})

	if err := r.Status().Update(ctx, &volume); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the BlockVolume controller with the manager.
// lookupPoolPhase returns (phase, found, err) for the named pool. It
// reads via the api when configured, otherwise falls back to the
// legacy CRD path on the local k8s apiserver.
func (r *BlockVolumeReconciler) lookupPoolPhase(
	ctx context.Context, name string,
) (string, bool, error) {
	if r.API != nil {
		p, err := r.API.GetPool(ctx, name)
		if err != nil {
			var apiErr *novanas.APIError
			if errors.As(err, &apiErr) && apiErr.Status == 404 {
				return "", false, nil
			}
			return "", false, err
		}
		return p.Status.Phase, true, nil
	}
	var pool novastorev1alpha1.StoragePool
	if err := r.Get(ctx, types.NamespacedName{Name: name}, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return pool.Status.Phase, true, nil
}

func (r *BlockVolumeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&novastorev1alpha1.BlockVolume{}).
		Named("blockvolume").
		Complete(r)
}
