package metrics

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

// fakePoolLister returns canned pool/status data so the ZFS collector
// can be exercised without spawning real zpool processes.
type fakePoolLister struct {
	pools  []pool.Pool
	detail map[string]*pool.Detail
	err    error
}

func (f *fakePoolLister) List(_ context.Context) ([]pool.Pool, error) {
	return f.pools, f.err
}

func (f *fakePoolLister) Get(_ context.Context, name string) (*pool.Detail, error) {
	if d, ok := f.detail[name]; ok {
		return d, nil
	}
	return nil, pool.ErrNotFound
}

type fakeDatasetLister struct{ rows []dataset.Dataset }

func (f *fakeDatasetLister) List(_ context.Context, _ string) ([]dataset.Dataset, error) {
	return f.rows, nil
}

// TestRegistration_AllExpectedNames scrapes a freshly built registry
// (after one ZFS poll) and asserts each expected metric name is present.
// This is the canary test for the contract documented in the task spec.
func TestRegistration_AllExpectedNames(t *testing.T) {
	m := New()

	pools := &fakePoolLister{
		pools: []pool.Pool{{
			Name: "tank", SizeBytes: 1 << 30, Allocated: 1 << 28,
			Free: (1 << 30) - (1 << 28), Health: "ONLINE",
			Capacity: 25, Fragmentation: 5,
		}},
		detail: map[string]*pool.Detail{
			"tank": {
				Pool: pool.Pool{Name: "tank"},
				Status: &pool.Status{
					State: "ONLINE",
					Scan:  &pool.ScrubStatus{State: "in-progress", ScannedBytes: 100, TotalBytes: 1000},
					Vdevs: []pool.Vdev{
						{Type: "mirror", Children: []pool.Vdev{
							{Type: "disk", Path: "/dev/sda", ReadErr: 1, WriteErr: 2, CksumErr: 3},
							{Type: "disk", Path: "/dev/sdb"},
						}},
					},
				},
			},
		},
	}
	dss := &fakeDatasetLister{rows: []dataset.Dataset{
		{Name: "tank", Type: "filesystem", UsedBytes: 100, AvailableBytes: 900},
		{Name: "tank/vol", Type: "volume", UsedBytes: 50, AvailableBytes: 0},
	}}

	zc := NewZFSCollector(slog.New(slog.NewTextHandler(io.Discard, nil)), pools, dss)
	zc.MustRegister(m.Registry)
	zc.poll(context.Background()) // synchronous single poll

	// Push one job metric so its families also appear in the scrape.
	m.Jobs.Dispatched("pool.create")
	m.Jobs.MarkRunning("pool.create")
	m.Jobs.MarkFinished("pool.create", "succeeded", 0.5)

	body := scrapeReg(t, m)

	expected := []string{
		// Job families with labels we just exercised.
		`nova_jobs_dispatched_total{kind="pool.create"} 1`,
		`nova_jobs_finished_total{kind="pool.create",state="succeeded"} 1`,
		`nova_jobs_in_flight{kind="pool.create"} 0`,
		"# HELP nova_job_duration_seconds",

		// ZFS pool gauges.
		`nova_zfs_pool_size_bytes{pool="tank"} 1.073741824e+09`,
		`nova_zfs_pool_alloc_bytes{pool="tank"}`,
		`nova_zfs_pool_free_bytes{pool="tank"}`,
		`nova_zfs_pool_capacity_pct{pool="tank"} 25`,
		`nova_zfs_pool_fragmentation_pct{pool="tank"} 5`,
		`nova_zfs_pool_health{pool="tank",state="ONLINE"} 1`,
		`nova_zfs_pool_health{pool="tank",state="DEGRADED"} 0`,
		`nova_zfs_pool_scrub_state{pool="tank",state="in-progress"} 1`,
		`nova_zfs_pool_scrub_scanned_bytes{pool="tank"} 100`,
		`nova_zfs_pool_scrub_total_bytes{pool="tank"} 1000`,

		// Dataset gauges.
		`nova_zfs_dataset_used_bytes{dataset="tank",type="filesystem"} 100`,
		`nova_zfs_dataset_available_bytes{dataset="tank/vol",type="volume"} 0`,

		// Vdev errors only for leaf disks.
		`nova_zfs_vdev_read_errors{path="/dev/sda",pool="tank",vdev="mirror"} 1`,
		`nova_zfs_vdev_write_errors{path="/dev/sda",pool="tank",vdev="mirror"} 2`,
		`nova_zfs_vdev_checksum_errors{path="/dev/sda",pool="tank",vdev="mirror"} 3`,

		// Standard collectors come from the prometheus default set.
		"go_goroutines",
		"process_cpu_seconds_total",
	}
	for _, e := range expected {
		if !strings.Contains(body, e) {
			t.Errorf("scrape missing %q", e)
		}
	}
}

// TestZFSCollector_PollErrorIsCounted asserts a List failure bumps the
// pollErrors counter and does not panic. It also confirms gauges from a
// previous successful poll are NOT cleared on failure (sticky-on-error
// semantics — see poll() doc comment).
func TestZFSCollector_PollErrorIsCounted(t *testing.T) {
	m := New()
	pools := &fakePoolLister{
		pools: []pool.Pool{{Name: "tank", SizeBytes: 42, Health: "ONLINE"}},
		detail: map[string]*pool.Detail{
			"tank": {Status: &pool.Status{State: "ONLINE"}},
		},
	}
	dss := &fakeDatasetLister{}

	zc := NewZFSCollector(slog.New(slog.NewTextHandler(io.Discard, nil)), pools, dss)
	zc.MustRegister(m.Registry)
	zc.poll(context.Background())

	// Now simulate a host-level zpool failure on the next poll.
	pools.err = io.ErrUnexpectedEOF
	zc.poll(context.Background())

	body := scrapeReg(t, m)
	if !strings.Contains(body, "nova_zfs_collector_errors_total 1") {
		t.Errorf("expected pollErrors counter to be 1, body:\n%s", body)
	}
	if !strings.Contains(body, `nova_zfs_pool_size_bytes{pool="tank"} 42`) {
		t.Errorf("expected previous successful poll's gauge to be retained")
	}
}

func scrapeReg(t *testing.T, m *Metrics) string {
	t.Helper()
	rr := newScrapeRecorder()
	m.Handler().ServeHTTP(rr, scrapeRequest())
	return rr.Body.String()
}
