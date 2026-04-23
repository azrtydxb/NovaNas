package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// SshKeyReconciler reconciles a SshKey object by projecting the public
// key line into a cluster-wide ConfigMap consumed by the node agent when
// rendering ~owner/.ssh/authorized_keys. The controller also parses the
// key to populate Status.KeyType and Status.Fingerprint (matching
// `ssh-keygen -lf`'s SHA256 format).
type SshKeyReconciler struct {
	reconciler.BaseReconciler
	AuthorizedKeysConfigMap string
	Recorder                record.EventRecorder
}

// parsePubKey extracts the key type and computes the SHA-256 fingerprint
// in the "SHA256:<base64>" form emitted by OpenSSH. Returns ("", "") on
// unparseable input rather than failing the reconcile; the fingerprint
// field is best-effort and should not block key projection.
func parsePubKey(line string) (keyType, fingerprint string) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		return "", ""
	}
	keyType = fields[0]
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return keyType, ""
	}
	sum := sha256.Sum256(blob)
	return keyType, "SHA256:" + strings.TrimRight(base64.StdEncoding.EncodeToString(sum[:]), "=")
}

// Reconcile ensures the SshKey entry in the authorized-keys ConfigMap.
func (r *SshKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "SshKey", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.SshKey
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("SshKey deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "ssh key removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerSshKey); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerSshKey); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Expiry handling: if past ExpiresAt, mark Expired and skip projection.
	if obj.Spec.ExpiresAt != nil && time.Now().After(obj.Spec.ExpiresAt.Time) {
		obj.Status.Phase = "Expired"
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation, "Expired", "key past expiry")
		if err := r.Client.Status().Update(ctx, &obj); err != nil && !apierrors.IsConflict(err) {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: defaultRequeue}, nil
	}

	cmName := r.AuthorizedKeysConfigMap
	if cmName == "" {
		cmName = "ssh-authorized-keys"
	}
	ns := obj.Namespace
	if ns == "" {
		ns = "novanas-system"
	}

	// Build the authorized_keys line. Owner is prepended as a comment so
	// per-owner splitting remains possible downstream.
	keyLine := strings.TrimSpace(obj.Spec.PublicKey)
	if obj.Spec.Comment != "" && keyLine != "" {
		// If the PublicKey itself doesn't already carry a comment, append ours.
		if parts := strings.Fields(keyLine); len(parts) < 3 {
			keyLine = keyLine + " " + obj.Spec.Comment
		}
	}

	cm := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: cmName}}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, &cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		if keyLine == "" {
			// Empty public key — keep the CR but don't project anything.
			delete(cm.Data, obj.Name)
			return nil
		}
		header := "# managed-by-novanas ssh key " + obj.Name + " owner=" + obj.Spec.Owner + "\n"
		cm.Data[obj.Name] = header + keyLine + "\n"
		return nil
	})
	if err != nil {
		result = "error"
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonChildEnsured, "authorized-keys configmap "+string(op))
	}

	// Populate typed status from the parsed key.
	keyType, fp := parsePubKey(obj.Spec.PublicKey)
	if keyType != "" {
		obj.Status.KeyType = keyType
	}
	if fp != "" {
		obj.Status.Fingerprint = fp
	} else if obj.Spec.PublicKey != "" && obj.Status.Fingerprint == "" {
		// Fallback: raw hex SHA256 of the whole line.
		h := sha256.Sum256([]byte(obj.Spec.PublicKey))
		obj.Status.Fingerprint = "sha256:" + hex.EncodeToString(h[:])
	}
	obj.Status.Phase = "Active"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation, reconciler.ReasonReconciled, "ssh key projected to "+cmName)
	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: defaultRequeue}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *SshKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "SshKey"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "sshkey-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.SshKey{}).
		Named("SshKey").
		Complete(r)
}
