package controllers

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
)

// ---------------- AlertChannel ----------------

func TestAlertChannelReconciler_ValidationFailure(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AlertChannel{
		ObjectMeta: newClusterMeta("bad"),
		Spec:       novanasv1alpha1.AlertChannelSpec{Type: "webhook"}, // missing url
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AlertChannelReconciler{BaseReconciler: newPart2Base(c, s, "AlertChannel"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("bad"))
	var got novanasv1alpha1.AlertChannel
	_ = c.Get(context.Background(), client.ObjectKey{Name: "bad"}, &got)
	if got.Status.Phase != "Failed" {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}
	if got.Status.ConsecutiveFailures == 0 {
		t.Fatalf("ConsecutiveFailures not incremented")
	}
}

func TestAlertChannelReconciler_SuspendedSkipsConfigMap(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AlertChannel{
		ObjectMeta: newClusterMeta("paused"),
		Spec: novanasv1alpha1.AlertChannelSpec{
			Type:      "email",
			Email:     &novanasv1alpha1.EmailChannelConfig{To: []string{"a@b"}},
			Suspended: true,
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AlertChannelReconciler{BaseReconciler: newPart2Base(c, s, "AlertChannel"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("paused"))
	var got novanasv1alpha1.AlertChannel
	_ = c.Get(context.Background(), client.ObjectKey{Name: "paused"}, &got)
	if got.Status.Phase != "Suspended" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestAlertChannelReconciler_SlackNeedsSecret(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AlertChannel{
		ObjectMeta: newClusterMeta("slack"),
		Spec: novanasv1alpha1.AlertChannelSpec{
			Type: "slack",
			Slack: &novanasv1alpha1.SlackChannelConfig{
				WebhookURLSecret: novanasv1alpha1.SecretKeyRef{Name: "slack-secret", Key: "url"},
			},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AlertChannelReconciler{BaseReconciler: newPart2Base(c, s, "AlertChannel"), Recorder: newPart2Recorder()}
	// First reconcile: secret absent -> Pending, not Failed.
	_, _ = r.Reconcile(context.Background(), part2Request("slack"))
	_, _ = r.Reconcile(context.Background(), part2Request("slack"))
	var got novanasv1alpha1.AlertChannel
	_ = c.Get(context.Background(), client.ObjectKey{Name: "slack"}, &got)
	if got.Status.Phase == "Failed" {
		t.Fatalf("phase should not be Failed when secret is missing, got %q", got.Status.Phase)
	}
}

// ---------------- AlertPolicy ----------------

func TestAlertPolicyReconciler_RuleHashStable(t *testing.T) {
	obj := &novanasv1alpha1.AlertPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec: novanasv1alpha1.AlertPolicySpec{
			Severity:  "warning",
			Condition: novanasv1alpha1.AlertCondition{Query: "up", Operator: "==", Threshold: "0"},
			Channels:  []string{"ops"},
		},
	}
	spec1 := renderPrometheusRuleSpec(obj)
	spec2 := renderPrometheusRuleSpec(obj)
	if hashRuleSpec(spec1) != hashRuleSpec(spec2) {
		t.Fatalf("hash is not stable across runs")
	}
}

func TestAlertPolicyReconciler_InvalidOperator(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.AlertPolicy{
		ObjectMeta: newClusterMeta("bad-op"),
		Spec: novanasv1alpha1.AlertPolicySpec{
			Severity:  "warning",
			Condition: novanasv1alpha1.AlertCondition{Query: "x", Operator: "=~", Threshold: "1"},
			Channels:  []string{"c"},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &AlertPolicyReconciler{BaseReconciler: newPart2Base(c, s, "AlertPolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("bad-op"))
	var got novanasv1alpha1.AlertPolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "bad-op"}, &got)
	if got.Status.Phase != "Failed" {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}
}

// ---------------- CloudBackupTarget ----------------

type failingProber struct{}

func (failingProber) Probe(_ context.Context, _ novanasv1alpha1.CloudBackupTargetSpec, _ map[string][]byte) (novanasv1alpha1.CloudBackupCapability, string, error) {
	return novanasv1alpha1.CloudBackupCapability{}, "", errProbe
}

var errProbe = &probeErr{"unreachable"}

type probeErr struct{ s string }

func (e *probeErr) Error() string { return e.s }

func TestCloudBackupTargetReconciler_UnreachableOnProbeFail(t *testing.T) {
	s := newPart2Scheme(t)
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "novanas-system"},
		Data:       map[string][]byte{"k": []byte("v")},
	}
	cr := &novanasv1alpha1.CloudBackupTarget{
		ObjectMeta: newClusterMeta("bad"),
		Spec: novanasv1alpha1.CloudBackupTargetSpec{
			Provider:          "s3",
			Bucket:            "b",
			CredentialsSecret: novanasv1alpha1.SecretKeyRef{Name: "c", Namespace: "novanas-system", Key: "k"},
		},
	}
	c := newPart2Client(s, []client.Object{cr, sec}, []client.Object{cr})
	r := &CloudBackupTargetReconciler{
		BaseReconciler: newPart2Base(c, s, "CloudBackupTarget"),
		Recorder:       newPart2Recorder(),
		Prober:         failingProber{},
	}
	mustReconcileOK(t, context.Background(), r, part2Request("bad"))
	var got novanasv1alpha1.CloudBackupTarget
	_ = c.Get(context.Background(), client.ObjectKey{Name: "bad"}, &got)
	if got.Status.Phase != "Unreachable" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.Reachable {
		t.Fatalf("Reachable should be false")
	}
	if got.Status.LastProbeError == "" {
		t.Fatalf("LastProbeError empty")
	}
}

func TestCloudBackupTargetReconciler_MissingSecretIsPending(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.CloudBackupTarget{
		ObjectMeta: newClusterMeta("t"),
		Spec: novanasv1alpha1.CloudBackupTargetSpec{
			Provider:          "s3",
			Bucket:            "b",
			CredentialsSecret: novanasv1alpha1.SecretKeyRef{Name: "missing", Namespace: "novanas-system", Key: "k"},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &CloudBackupTargetReconciler{BaseReconciler: newPart2Base(c, s, "CloudBackupTarget"), Recorder: newPart2Recorder(), Prober: stubProber{}}
	mustReconcileOK(t, context.Background(), r, part2Request("t"))
	var got novanasv1alpha1.CloudBackupTarget
	_ = c.Get(context.Background(), client.ObjectKey{Name: "t"}, &got)
	if got.Status.Phase != "Pending" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

// ---------------- CloudBackupJob ----------------

func TestCloudBackupJobCron_NextRunParses(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := cronNextRun("0 2 * * *", from)
	if err != nil {
		t.Fatalf("cron: %v", err)
	}
	if next.Hour() != 2 || next.Minute() != 0 {
		t.Fatalf("unexpected next=%v", next)
	}
	if !next.After(from) {
		t.Fatalf("next not after from")
	}
}

func TestCloudBackupJobCron_Weekly(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) // Wednesday
	// Every Sunday at 03:30 -> Jan 5 2025.
	next, err := cronNextRun("30 3 * * 0", from)
	if err != nil {
		t.Fatalf("cron: %v", err)
	}
	if next.Weekday() != time.Sunday || next.Hour() != 3 || next.Minute() != 30 {
		t.Fatalf("unexpected next=%v", next)
	}
}

func TestCloudBackupJobCron_Invalid(t *testing.T) {
	if _, err := cronNextRun("bad cron", time.Now()); err == nil {
		t.Fatalf("expected error")
	}
}

// ---------------- SLO ----------------

func TestSLOReconciler_BreachedWhenBurnRateHigh(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ServiceLevelObjective{
		ObjectMeta: newClusterMeta("breached-slo"),
		Spec: novanasv1alpha1.ServiceLevelObjectiveSpec{
			Target: "99.9",
			Window: "30d",
			Indicator: novanasv1alpha1.SLOIndicator{
				GoodQuery:  "sum(rate(good_requests_total[5m]))",
				TotalQuery: "sum(rate(requests_total[5m]))",
			},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	// Heavy burn: 50% failures with 0.1% budget.
	r := &ServiceLevelObjectiveReconciler{
		BaseReconciler: newPart2Base(c, s, "ServiceLevelObjective"),
		Recorder:       newPart2Recorder(),
		Prom:           stubPromClient{good: 500, total: 1000},
	}
	mustReconcileOK(t, context.Background(), r, part2Request("breached-slo"))
	var got novanasv1alpha1.ServiceLevelObjective
	_ = c.Get(context.Background(), client.ObjectKey{Name: "breached-slo"}, &got)
	if got.Status.Phase != "Breached" {
		t.Fatalf("phase = %q, want Breached (burn rate should be >= 14.4)", got.Status.Phase)
	}
	if got.Status.CurrentSLI == "" {
		t.Fatalf("CurrentSLI unset")
	}
}

func TestSLOReconciler_ValidationFailure(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ServiceLevelObjective{
		ObjectMeta: newClusterMeta("bad-slo"),
		Spec:       novanasv1alpha1.ServiceLevelObjectiveSpec{Target: "101", Window: "30d"},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ServiceLevelObjectiveReconciler{BaseReconciler: newPart2Base(c, s, "ServiceLevelObjective"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("bad-slo"))
	var got novanasv1alpha1.ServiceLevelObjective
	_ = c.Get(context.Background(), client.ObjectKey{Name: "bad-slo"}, &got)
	if got.Status.Phase != "Failed" {
		t.Fatalf("phase = %q, want Failed", got.Status.Phase)
	}
}
