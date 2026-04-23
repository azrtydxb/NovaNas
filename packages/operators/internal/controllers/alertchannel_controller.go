package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

const finalizerAlertChannel = reconciler.FinalizerPrefix + "alertchannel"

// AlertChannelReconciler validates channel configuration, resolves the
// Secret references used by downstream dispatchers, and publishes a
// ConfigMap that the in-cluster alertmanager-sidecar reads.
//
// The dispatcher itself lives out-of-process; this controller's job is
// purely to validate + materialise config.
type AlertChannelReconciler struct {
	reconciler.BaseReconciler
	Recorder record.EventRecorder
}

// Reconcile ensures the channel ConfigMap and the status fields
// (Phase, ObservedGeneration, ConsecutiveFailures, ResolvedSecretRef).
func (r *AlertChannelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx).WithValues("controller", "AlertChannel", "key", req.NamespacedName)
	result := "ok"
	defer func() { r.ObserveReconcile(start, result) }()

	var obj novanasv1alpha1.AlertChannel
	if err := r.Client.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonDeleted, "AlertChannel removed")
		if err := reconciler.RemoveFinalizer(ctx, r.Client, &obj, finalizerAlertChannel); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if added, err := reconciler.EnsureFinalizer(ctx, r.Client, &obj, finalizerAlertChannel); err != nil {
		result = "error"
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{Requeue: true}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation,
		reconciler.ReasonReconciling, "validating alert channel")
	obj.Status.Phase = "Pending"

	// --- validation --------------------------------------------------
	if err := validateAlertChannel(&obj.Spec); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.LastError = err.Error()
		obj.Status.ConsecutiveFailures++
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonValidationFailed, err.Error())
		reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonFailed, err.Error())
		_ = statusUpdate(ctx, r.Client, &obj)
		result = "error"
		// Validation failures are terminal until spec changes; slow requeue.
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if obj.Spec.Suspended {
		obj.Status.Phase = "Suspended"
		obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
			reconciler.ReasonReconciled, "channel suspended")
		if err := statusUpdate(ctx, r.Client, &obj); err != nil {
			result = "error"
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
	}

	// --- resolve primary secret (for status visibility) --------------
	primary := primarySecretRef(&obj.Spec)
	if primary != nil {
		ns := primary.Namespace
		if ns == "" {
			ns = "novanas-system"
		}
		var sec corev1.Secret
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: primary.Name}, &sec); err != nil {
			if !apierrors.IsNotFound(err) {
				result = "error"
				return ctrl.Result{}, err
			}
			// Secret missing — not a hard error; the dispatcher will retry.
			obj.Status.Phase = "Pending"
			obj.Status.LastError = fmt.Sprintf("secret %s/%s not yet available", ns, primary.Name)
			obj.Status.Conditions = reconciler.MarkProgressing(obj.Status.Conditions, obj.Generation,
				reconciler.ReasonAwaitingExternal, obj.Status.LastError)
			if err := statusUpdate(ctx, r.Client, &obj); err != nil {
				result = "error"
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if _, ok := sec.Data[primary.Key]; !ok {
			err := fmt.Errorf("secret %s/%s missing key %q", ns, primary.Name, primary.Key)
			obj.Status.Phase = "Failed"
			obj.Status.LastError = err.Error()
			obj.Status.ConsecutiveFailures++
			obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
				reconciler.ReasonValidationFailed, err.Error())
			reconciler.EmitWarning(r.Recorder, &obj, reconciler.EventReasonFailed, err.Error())
			_ = statusUpdate(ctx, r.Client, &obj)
			result = "error"
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		obj.Status.ResolvedSecretRef = &corev1.LocalObjectReference{Name: primary.Name}
	}

	// --- materialise dispatcher ConfigMap ----------------------------
	data := alertChannelConfigData(&obj)
	if _, err := ensureConfigMap(ctx, r.Client, "novanas-system",
		childName(obj.Name, "alert-channel"), &obj,
		data,
		map[string]string{"novanas.io/kind": "AlertChannel"}); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.LastError = err.Error()
		obj.Status.ConsecutiveFailures++
		obj.Status.Conditions = reconciler.MarkFailed(obj.Status.Conditions, obj.Generation,
			"ConfigMapFailed", err.Error())
		_ = statusUpdate(ctx, r.Client, &obj)
		result = "error"
		return ctrl.Result{}, err
	}

	// --- success -----------------------------------------------------
	obj.Status.ConsecutiveFailures = 0
	obj.Status.LastError = ""
	obj.Status.Phase = "Active"
	obj.Status.Conditions = reconciler.MarkReady(obj.Status.Conditions, obj.Generation,
		reconciler.ReasonReconciled, "alert channel ready")
	logger.V(1).Info("alert channel ready", "type", obj.Spec.Type)
	if err := statusUpdate(ctx, r.Client, &obj); err != nil {
		result = "error"
		return ctrl.Result{}, err
	}
	reconciler.Emit(r.Recorder, &obj, reconciler.EventReasonReady, "AlertChannel active")
	return ctrl.Result{RequeueAfter: defaultRequeuePart2}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *AlertChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ControllerName = "AlertChannel"
	if r.Recorder == nil {
		r.Recorder = reconciler.NewRecorder(mgr, "alertchannel-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&novanasv1alpha1.AlertChannel{}).
		Named("AlertChannel").
		Complete(r)
}

// validateAlertChannel enforces the discriminated-union shape of the spec.
func validateAlertChannel(spec *novanasv1alpha1.AlertChannelSpec) error {
	if spec.Type == "" {
		return fmt.Errorf("spec.type is required")
	}
	switch spec.Type {
	case "email":
		if spec.Email == nil || len(spec.Email.To) == 0 {
			return fmt.Errorf("spec.email.to must be non-empty for type=email")
		}
	case "webhook":
		if spec.Webhook == nil || spec.Webhook.URL == "" {
			return fmt.Errorf("spec.webhook.url is required for type=webhook")
		}
		if !strings.HasPrefix(spec.Webhook.URL, "http://") && !strings.HasPrefix(spec.Webhook.URL, "https://") {
			return fmt.Errorf("spec.webhook.url must be http(s)")
		}
	case "slack":
		if spec.Slack == nil || spec.Slack.WebhookURLSecret.Name == "" {
			return fmt.Errorf("spec.slack.webhookUrlSecret is required for type=slack")
		}
	case "pagerduty":
		if spec.PagerDuty == nil || spec.PagerDuty.IntegrationKeySecret.Name == "" {
			return fmt.Errorf("spec.pagerduty.integrationKeySecret is required for type=pagerduty")
		}
	case "ntfy":
		if spec.Ntfy == nil || spec.Ntfy.Topic == "" {
			return fmt.Errorf("spec.ntfy.topic is required for type=ntfy")
		}
	case "pushover":
		if spec.Pushover == nil || spec.Pushover.UserKeySecret.Name == "" || spec.Pushover.TokenSecret.Name == "" {
			return fmt.Errorf("spec.pushover.userKeySecret and tokenSecret are required")
		}
	case "discord", "telegram":
		// Accepted for forward-compat; dispatcher handles wire format.
	default:
		return fmt.Errorf("unsupported channel type %q", spec.Type)
	}
	return nil
}

// primarySecretRef returns the main credentials SecretKeyRef for the channel, if any.
func primarySecretRef(spec *novanasv1alpha1.AlertChannelSpec) *novanasv1alpha1.SecretKeyRef {
	switch spec.Type {
	case "webhook":
		if spec.Webhook != nil && spec.Webhook.SecretRef != nil {
			return spec.Webhook.SecretRef
		}
	case "slack":
		if spec.Slack != nil {
			r := spec.Slack.WebhookURLSecret
			return &r
		}
	case "pagerduty":
		if spec.PagerDuty != nil {
			r := spec.PagerDuty.IntegrationKeySecret
			return &r
		}
	case "ntfy":
		if spec.Ntfy != nil && spec.Ntfy.AuthSecret != nil {
			return spec.Ntfy.AuthSecret
		}
	case "pushover":
		if spec.Pushover != nil {
			r := spec.Pushover.UserKeySecret
			return &r
		}
	case "email":
		if spec.Email != nil && spec.Email.PasswordSecret != nil {
			return spec.Email.PasswordSecret
		}
	}
	return nil
}

// alertChannelConfigData flattens the spec into key/value pairs the
// alert dispatcher consumes.
func alertChannelConfigData(obj *novanasv1alpha1.AlertChannel) map[string]string {
	data := map[string]string{
		"channel":     obj.Name,
		"type":        string(obj.Spec.Type),
		"minSeverity": obj.Spec.MinSeverity,
	}
	if obj.Spec.RateLimitPerMinute > 0 {
		data["rateLimitPerMinute"] = fmt.Sprintf("%d", obj.Spec.RateLimitPerMinute)
	}
	switch obj.Spec.Type {
	case "email":
		if obj.Spec.Email != nil {
			data["email.to"] = strings.Join(obj.Spec.Email.To, ",")
			data["email.from"] = obj.Spec.Email.From
			data["email.smtpServer"] = obj.Spec.Email.SmtpServer
			if obj.Spec.Email.SmtpPort > 0 {
				data["email.smtpPort"] = fmt.Sprintf("%d", obj.Spec.Email.SmtpPort)
			}
			data["email.startTls"] = fmt.Sprintf("%t", obj.Spec.Email.StartTLS)
		}
	case "webhook":
		if obj.Spec.Webhook != nil {
			data["webhook.url"] = obj.Spec.Webhook.URL
			if obj.Spec.Webhook.Method != "" {
				data["webhook.method"] = obj.Spec.Webhook.Method
			}
		}
	case "slack":
		if obj.Spec.Slack != nil {
			data["slack.channel"] = obj.Spec.Slack.Channel
			data["slack.username"] = obj.Spec.Slack.Username
		}
	case "pagerduty":
		if obj.Spec.PagerDuty != nil {
			data["pagerduty.severity"] = obj.Spec.PagerDuty.Severity
			data["pagerduty.component"] = obj.Spec.PagerDuty.Component
		}
	case "ntfy":
		if obj.Spec.Ntfy != nil {
			data["ntfy.server"] = obj.Spec.Ntfy.Server
			data["ntfy.topic"] = obj.Spec.Ntfy.Topic
			data["ntfy.priority"] = obj.Spec.Ntfy.Priority
		}
	case "pushover":
		if obj.Spec.Pushover != nil {
			data["pushover.device"] = obj.Spec.Pushover.Device
			if obj.Spec.Pushover.Priority != 0 {
				data["pushover.priority"] = fmt.Sprintf("%d", obj.Spec.Pushover.Priority)
			}
		}
	}
	return data
}
