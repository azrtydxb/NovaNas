package controllers

import (
	"context"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
)

// stubProber is a test double for CloudProber. It always reports the
// target as reachable with a minimal capability set and echoes back a
// synthetic resolved endpoint so tests can assert on it.
type stubProber struct{}

func (stubProber) Probe(_ context.Context, spec novanasv1alpha1.CloudBackupTargetSpec, _ map[string][]byte) (novanasv1alpha1.CloudBackupCapability, string, error) {
	return novanasv1alpha1.CloudBackupCapability{MultipartUpload: true, Versioning: true},
		"https://stub/" + spec.Bucket, nil
}

// stubPromClient returns a deterministic SLI from an injected
// good/total pair. Used by SLO tests to avoid a real Prometheus.
type stubPromClient struct {
	good  float64
	total float64
	err   error
}

func (s stubPromClient) Instant(_ context.Context, _ string, q string) (float64, error) {
	if s.err != nil {
		return 0, s.err
	}
	// Crude routing: the SLO controller issues GoodQuery first then
	// TotalQuery. We can't match on content so return good on the
	// first call and total on the second, using a shared channel —
	// but stateless is simpler: treat any query whose text contains
	// "good" keyword as good; otherwise total.
	if containsAny(q, "good", "success", "!~\"5") {
		return s.good, nil
	}
	return s.total, nil
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if indexOf(s, n) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}
