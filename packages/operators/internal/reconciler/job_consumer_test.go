package reconciler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// fakeBackend yields one job then nothing, and records Complete calls.
type fakeBackend struct {
	mu       sync.Mutex
	yielded  bool
	jobID    string
	kind     string
	input    map[string]any
	complete chan reconciler.JobResult
}

func (b *fakeBackend) ClaimNext(_ context.Context, kind string) (reconciler.JobRecord, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.yielded || kind != b.kind {
		return reconciler.JobRecord{}, false, nil
	}
	b.yielded = true
	return reconciler.JobRecord{ID: b.jobID, Kind: kind, State: "running", Input: b.input}, true, nil
}

func (b *fakeBackend) Complete(_ context.Context, _ string, r reconciler.JobResult) error {
	b.complete <- r
	return nil
}

func TestJobConsumer_DispatchesAndCompletes(t *testing.T) {
	be := &fakeBackend{
		jobID:    "job-1",
		kind:     "test.kind",
		input:    map[string]any{"x": "y"},
		complete: make(chan reconciler.JobResult, 1),
	}
	c := reconciler.NewJobConsumer(be, logr.Discard())
	c.Interval = 10 * time.Millisecond
	c.Register("test.kind", func(_ context.Context, j reconciler.JobRecord) reconciler.JobResult {
		return reconciler.JobResult{Success: true, Message: "ok: " + j.ID}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case r := <-be.complete:
		if !r.Success || r.Message != "ok: job-1" {
			t.Fatalf("unexpected result: %+v", r)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for Complete")
	}
}
