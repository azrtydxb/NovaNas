package controllers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// CloudProber abstracts the per-provider reachability probe so tests can
// inject a stub without bringing in cloud SDKs.
type CloudProber interface {
	// Probe returns a capability snapshot and an optional resolved
	// endpoint string. A non-nil error means the target is unreachable.
	Probe(ctx context.Context, spec novanasv1alpha1.CloudBackupTargetSpec, creds map[string][]byte) (novanasv1alpha1.CloudBackupCapability, string, error)
}

// CloudBackupTargetReconciler manages a CloudBackupTarget CR.
//
// Reconcile loop:
//  1. Validate spec (provider, bucket, credential Secret).
//  2. Load credentials from Secret.
//  3. Probe the target through the injected CloudProber. Prober
//     implements provider-specific reachability checks (S3 HeadBucket
//     via REST, Azure Blob GetProperties via REST). On failure we
//     record LastProbeError and mark Phase=Unreachable.
//  4. Cache detected capabilities + resolved endpoint in .status.
type CloudBackupTargetReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
	// Prober defaults to httpCloudProber when nil.
	Prober CloudProber
}

// Reconcile ensures finalizer, probes reachability, and updates status.
func (r *CloudBackupTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "CloudBackupTarget", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.CloudBackupTarget
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		logger.Info("CloudBackupTarget deleting")
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "cloud backup target removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCloudBackupTarget); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, reconciler.FinalizerCloudBackupTarget); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Phase = "Probing"
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation,
		reconciler.ReasonReconciling, "probing target reachability")

	if err := validateCloudBackupTarget(&obj.Spec); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.LastProbeError = err.Error()
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonValidationFailed, err.Error())
		reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonFailed, err.Error())
		_ = r.Client.Status().Update(ctx, &obj)
		result = "error"
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// Load the credentials Secret. Missing secret is "AwaitingExternal".
	credSec := corev1.Secret{}
	credNS := obj.Spec.CredentialsSecret.Namespace
	if credNS == "" {
		credNS = "novanas-system"
	}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: credNS, Name: obj.Spec.CredentialsSecret.Name}, &credSec); err != nil {
		if apierrors.IsNotFound(err) {
			obj.Status.Phase = "Pending"
			obj.Status.LastProbeError = fmt.Sprintf("credentials secret %s/%s not found", credNS, obj.Spec.CredentialsSecret.Name)
			obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation,
				reconciler.ReasonAwaitingExternal, obj.Status.LastProbeError)
			_ = r.Client.Status().Update(ctx, &obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}

	prober := r.Prober
	if prober == nil {
		prober = defaultHTTPProber()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	caps, resolved, probeErr := prober.Probe(probeCtx, obj.Spec, credSec.Data)

	now := metav1.NewTime(time.Now())
	obj.Status.LastProbeAt = &now
	obj.Status.ResolvedEndpoint = resolved

	if probeErr != nil {
		obj.Status.Reachable = false
		obj.Status.Phase = "Unreachable"
		obj.Status.LastProbeError = probeErr.Error()
		obj.Status.Conditions = reconciler.MarkDegraded(obj.Status.Conditions, obj.Generation,
			"ProbeFailed", probeErr.Error())
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			"ProbeFailed", probeErr.Error())
		reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonExternalSync, probeErr.Error())
		if err := r.Client.Status().Update(ctx, &obj); err != nil && !apierrors.IsConflict(err) {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.probeInterval(&obj.Spec)}, nil
	}

	obj.Status.Reachable = true
	obj.Status.LastProbeError = ""
	c := caps
	obj.Status.Capabilities = &c
	obj.Status.Phase = "Ready"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
		reconciler.ReasonReconciled, "target reachable")
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "CloudBackupTarget reachable")

	if err := r.Client.Status().Update(ctx, &obj); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		result = "error"
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: r.probeInterval(&obj.Spec)}, nil
}

func (r *CloudBackupTargetReconciler) probeInterval(spec *novanasv1alpha1.CloudBackupTargetSpec) time.Duration {
	if spec.ReachabilityProbeIntervalSeconds > 0 {
		return time.Duration(spec.ReachabilityProbeIntervalSeconds) * time.Second
	}
	return 5 * time.Minute
}

// SetupWithManager registers the controller with the manager.
func (r *CloudBackupTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "CloudBackupTarget"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "cloudbackuptarget-controller")
	}
	if r.Prober == nil {
		r.Prober = defaultHTTPProber()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.CloudBackupTarget{}).
		Named("CloudBackupTarget").
		Complete(r)
}

func validateCloudBackupTarget(spec *novanasv1alpha1.CloudBackupTargetSpec) error {
	switch spec.Provider {
	case "s3", "b2", "azure", "gcs", "swift":
	case "":
		return fmt.Errorf("spec.provider is required")
	default:
		return fmt.Errorf("unsupported provider %q", spec.Provider)
	}
	if spec.Bucket == "" {
		return fmt.Errorf("spec.bucket is required")
	}
	if spec.CredentialsSecret.Name == "" {
		return fmt.Errorf("spec.credentialsSecret.name is required")
	}
	return nil
}

// --- HTTP prober ---------------------------------------------------------

// httpCloudProber issues the most lightweight reachability probe possible
// per provider using only net/http:
//   * S3:   HEAD https://{endpoint or <region>.amazonaws.com}/{bucket}
//   * B2:   HEAD https://s3.{region}.backblazeb2.com/{bucket}
//   * Azure: GET  https://{account}.blob.core.windows.net/{bucket}?restype=container (anonymous; auth failure is still "reachable")
//   * GCS:  HEAD https://storage.googleapis.com/{bucket}
//   * swift: HEAD {endpoint}/v1/AUTH_.../{bucket}
//
// This is intentionally not an authenticated probe; full auth checking
// requires provider SDKs we do not vendor. Reachability + DNS + TCP +
// TLS-handshake is a solid first signal. The backup engine (restic /
// kopia) will fail with a clearer error at backup time if auth is bad.
type httpCloudProber struct{ client *http.Client }

func defaultHTTPProber() *httpCloudProber {
	return &httpCloudProber{client: &http.Client{Timeout: 10 * time.Second}}
}

func (p *httpCloudProber) Probe(ctx context.Context, spec novanasv1alpha1.CloudBackupTargetSpec, creds map[string][]byte) (novanasv1alpha1.CloudBackupCapability, string, error) {
	u, caps, err := resolveProbeURL(spec, creds)
	if err != nil {
		return novanasv1alpha1.CloudBackupCapability{}, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return caps, u, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return caps, u, fmt.Errorf("probe %s: %w", u, err)
	}
	_ = resp.Body.Close()
	// 2xx / 3xx / 401 / 403 all indicate the endpoint is reachable.
	// 404 on a non-existent bucket is unreachable for our purposes.
	switch {
	case resp.StatusCode < 400:
		return caps, u, nil
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return caps, u, nil
	default:
		return caps, u, fmt.Errorf("probe %s: HTTP %d", u, resp.StatusCode)
	}
}

func resolveProbeURL(spec novanasv1alpha1.CloudBackupTargetSpec, creds map[string][]byte) (string, novanasv1alpha1.CloudBackupCapability, error) {
	caps := novanasv1alpha1.CloudBackupCapability{MultipartUpload: true}
	switch spec.Provider {
	case "s3":
		caps.Versioning = true
		caps.ObjectLock = true
		caps.ServerSideCrypt = true
		caps.LifecyclePolicy = true
		endpoint := spec.Endpoint
		if endpoint == "" {
			region := spec.Region
			if region == "" {
				region = "us-east-1"
			}
			endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", region)
		}
		u, err := url.Parse(endpoint)
		if err != nil {
			return "", caps, err
		}
		u.Path = "/" + spec.Bucket
		return u.String(), caps, nil
	case "b2":
		caps.LifecyclePolicy = true
		region := spec.Region
		if region == "" {
			region = "us-west-002"
		}
		return fmt.Sprintf("https://s3.%s.backblazeb2.com/%s", region, spec.Bucket), caps, nil
	case "azure":
		caps.Versioning = true
		caps.ServerSideCrypt = true
		caps.LifecyclePolicy = true
		account := string(creds["AZURE_ACCOUNT_NAME"])
		if account == "" {
			account = string(creds["account"])
		}
		if account == "" {
			return "", caps, fmt.Errorf("azure provider requires AZURE_ACCOUNT_NAME in credentialsSecret")
		}
		host := spec.Endpoint
		if host == "" {
			host = fmt.Sprintf("https://%s.blob.core.windows.net", account)
		}
		return fmt.Sprintf("%s/%s?restype=container", host, spec.Bucket), caps, nil
	case "gcs":
		caps.Versioning = true
		caps.ServerSideCrypt = true
		caps.LifecyclePolicy = true
		return fmt.Sprintf("https://storage.googleapis.com/%s", spec.Bucket), caps, nil
	case "swift":
		host := spec.Endpoint
		if host == "" {
			return "", caps, fmt.Errorf("swift provider requires spec.endpoint")
		}
		return fmt.Sprintf("%s/%s", host, spec.Bucket), caps, nil
	default:
		return "", caps, fmt.Errorf("unsupported provider %q", spec.Provider)
	}
}
