package metrics

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

// PoolLister is the subset of pool.Manager used by the ZFS collector. We
// take the interface (rather than the concrete *pool.Manager) so tests can
// inject fakes without spawning real zpool processes.
type PoolLister interface {
	List(ctx context.Context) ([]pool.Pool, error)
	Get(ctx context.Context, name string) (*pool.Detail, error)
}

// DatasetLister is the subset of dataset.Manager used by the ZFS collector.
type DatasetLister interface {
	List(ctx context.Context, root string) ([]dataset.Dataset, error)
}

// ZFSCollector polls the pool and dataset managers periodically and
// exposes the result as Prometheus gauges. It implements the polling
// model rather than prometheus.Collector's collect-on-scrape model so
// shelling out to zpool/zfs binaries is decoupled from the latency of a
// /metrics scrape: a slow zpool status will not stall the scrape.
type ZFSCollector struct {
	logger   *slog.Logger
	pools    PoolLister
	datasets DatasetLister
	interval time.Duration

	// All gauges are vector-typed and re-Set on each poll. We Reset
	// before each rebuild so vanishing pools/datasets do not retain
	// stale samples.
	mu sync.Mutex

	poolSize       *prometheus.GaugeVec
	poolAlloc      *prometheus.GaugeVec
	poolFree       *prometheus.GaugeVec
	poolCapacity   *prometheus.GaugeVec
	poolFrag       *prometheus.GaugeVec
	poolHealth     *prometheus.GaugeVec
	poolScrubState *prometheus.GaugeVec
	poolScanned    *prometheus.GaugeVec
	poolScanTotal  *prometheus.GaugeVec

	dsUsed  *prometheus.GaugeVec
	dsAvail *prometheus.GaugeVec

	vdevReadErr  *prometheus.GaugeVec
	vdevWriteErr *prometheus.GaugeVec
	vdevCksumErr *prometheus.GaugeVec

	// pollErrors counts polls that failed to enumerate pools or any
	// individual pool's status. Operators can alert on a non-zero rate.
	pollErrors prometheus.Counter

	// healthStates is the closed set of states we surface in poolHealth.
	// ZFS itself uses ONLINE/DEGRADED/FAULTED/OFFLINE/REMOVED/UNAVAIL/SUSPENDED.
	// Listing the set explicitly keeps cardinality predictable.
	healthStates []string

	// scrubStates mirrors pool.ScrubStatus.State. "none" appears when no
	// scan has been recorded.
	scrubStates []string
}

// NewZFSCollector builds a collector wired to the given managers. The
// poll interval is fixed at 30s by default; callers can override via the
// returned struct's exported field before starting Run.
func NewZFSCollector(logger *slog.Logger, pools PoolLister, datasets DatasetLister) *ZFSCollector {
	c := &ZFSCollector{
		logger:   logger,
		pools:    pools,
		datasets: datasets,
		interval: 30 * time.Second,

		poolSize: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_size_bytes",
			Help: "Pool total size in bytes (zpool list 'size').",
		}, []string{"pool"}),
		poolAlloc: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_alloc_bytes",
			Help: "Pool allocated bytes (zpool list 'allocated').",
		}, []string{"pool"}),
		poolFree: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_free_bytes",
			Help: "Pool free bytes (zpool list 'free').",
		}, []string{"pool"}),
		poolCapacity: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_capacity_pct",
			Help: "Pool capacity as a percentage (0-100).",
		}, []string{"pool"}),
		poolFrag: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_fragmentation_pct",
			Help: "Pool fragmentation as a percentage (0-100).",
		}, []string{"pool"}),
		poolHealth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_health",
			Help: "Pool health: 1 when state==label, 0 otherwise.",
		}, []string{"pool", "state"}),
		poolScrubState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_scrub_state",
			Help: "Pool scrub state: 1 when current scan state==label, 0 otherwise.",
		}, []string{"pool", "state"}),
		poolScanned: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_scrub_scanned_bytes",
			Help: "Bytes scanned in the current/last scrub.",
		}, []string{"pool"}),
		poolScanTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_pool_scrub_total_bytes",
			Help: "Total bytes the current/last scrub plans to scan.",
		}, []string{"pool"}),

		dsUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_dataset_used_bytes",
			Help: "Dataset 'used' property in bytes.",
		}, []string{"dataset", "type"}),
		dsAvail: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_dataset_available_bytes",
			Help: "Dataset 'available' property in bytes.",
		}, []string{"dataset", "type"}),

		vdevReadErr: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_vdev_read_errors",
			Help: "Read errors reported by zpool status, summed at the leaf vdev level.",
		}, []string{"pool", "vdev", "path"}),
		vdevWriteErr: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_vdev_write_errors",
			Help: "Write errors reported by zpool status, summed at the leaf vdev level.",
		}, []string{"pool", "vdev", "path"}),
		vdevCksumErr: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nova_zfs_vdev_checksum_errors",
			Help: "Checksum errors reported by zpool status, summed at the leaf vdev level.",
		}, []string{"pool", "vdev", "path"}),

		pollErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nova_zfs_collector_errors_total",
			Help: "Polls that failed to enumerate pools or a pool's status.",
		}),

		healthStates: []string{"ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "REMOVED", "UNAVAIL", "SUSPENDED"},
		scrubStates:  []string{"none", "in-progress", "finished", "resilver"},
	}
	return c
}

// MustRegister attaches all gauges to reg. Call before Run starts.
func (c *ZFSCollector) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(
		c.poolSize, c.poolAlloc, c.poolFree, c.poolCapacity, c.poolFrag,
		c.poolHealth, c.poolScrubState, c.poolScanned, c.poolScanTotal,
		c.dsUsed, c.dsAvail,
		c.vdevReadErr, c.vdevWriteErr, c.vdevCksumErr,
		c.pollErrors,
	)
}

// SetInterval overrides the default 30s poll cadence. Tests use this to
// drive the collector at sub-second intervals.
func (c *ZFSCollector) SetInterval(d time.Duration) {
	if d > 0 {
		c.interval = d
	}
}

// Run polls until ctx is cancelled. The first poll fires immediately so
// /metrics has data on the first scrape after startup.
func (c *ZFSCollector) Run(ctx context.Context) {
	c.poll(ctx)
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.poll(ctx)
		}
	}
}

// poll runs one full pool+dataset enumeration and rewrites the gauges.
//
// On error we increment pollErrors and log at warn — we do NOT clear the
// gauges so a transient zpool failure doesn't briefly empty the metrics.
// The next successful poll fully replaces them.
func (c *ZFSCollector) poll(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	pools, err := c.pools.List(ctx)
	if err != nil {
		c.pollErrors.Inc()
		if c.logger != nil {
			c.logger.Warn("zfs metrics: pool list failed", "err", err)
		}
		return
	}

	// Reset everything pool-scoped so disappearing pools don't leak.
	c.poolSize.Reset()
	c.poolAlloc.Reset()
	c.poolFree.Reset()
	c.poolCapacity.Reset()
	c.poolFrag.Reset()
	c.poolHealth.Reset()
	c.poolScrubState.Reset()
	c.poolScanned.Reset()
	c.poolScanTotal.Reset()
	c.vdevReadErr.Reset()
	c.vdevWriteErr.Reset()
	c.vdevCksumErr.Reset()

	for _, p := range pools {
		c.poolSize.WithLabelValues(p.Name).Set(float64(p.SizeBytes))
		c.poolAlloc.WithLabelValues(p.Name).Set(float64(p.Allocated))
		c.poolFree.WithLabelValues(p.Name).Set(float64(p.Free))
		c.poolCapacity.WithLabelValues(p.Name).Set(float64(p.Capacity))
		c.poolFrag.WithLabelValues(p.Name).Set(float64(p.Fragmentation))

		// Emit a health gauge for every known state so PromQL can
		// compare label==state without guessing which states are
		// observable. The pool.Health string comes from `zpool list -p`
		// in upper-case.
		for _, s := range c.healthStates {
			v := 0.0
			if s == p.Health {
				v = 1.0
			}
			c.poolHealth.WithLabelValues(p.Name, s).Set(v)
		}

		// Pull status separately. ListPools alone doesn't give us scan
		// or vdev-error info — those come from `zpool status`.
		detail, err := c.pools.Get(ctx, p.Name)
		if err != nil {
			// Pool vanished between List and Get, or `zpool status`
			// errored. Increment but keep going — we still want metrics
			// for the other pools and the data we got from list.
			if !errors.Is(err, pool.ErrNotFound) {
				c.pollErrors.Inc()
				if c.logger != nil {
					c.logger.Warn("zfs metrics: pool status failed", "pool", p.Name, "err", err)
				}
			}
			continue
		}
		c.recordScrub(p.Name, detail.Status)
		c.recordVdevs(p.Name, detail.Status)
	}

	// Datasets is best-effort. A failure here is logged but does not
	// nuke the per-pool gauges we already wrote.
	datasets, err := c.datasets.List(ctx, "")
	if err != nil {
		c.pollErrors.Inc()
		if c.logger != nil {
			c.logger.Warn("zfs metrics: dataset list failed", "err", err)
		}
		return
	}
	c.dsUsed.Reset()
	c.dsAvail.Reset()
	for _, d := range datasets {
		c.dsUsed.WithLabelValues(d.Name, d.Type).Set(float64(d.UsedBytes))
		c.dsAvail.WithLabelValues(d.Name, d.Type).Set(float64(d.AvailableBytes))
	}
}

// recordScrub maps the parsed scrub state onto the scrubStates label
// space (one gauge per known state, exactly one of which is 1).
func (c *ZFSCollector) recordScrub(poolName string, st *pool.Status) {
	if st == nil {
		return
	}
	current := ""
	if st.Scan != nil {
		current = st.Scan.State
		c.poolScanned.WithLabelValues(poolName).Set(float64(st.Scan.ScannedBytes))
		c.poolScanTotal.WithLabelValues(poolName).Set(float64(st.Scan.TotalBytes))
	}
	for _, s := range c.scrubStates {
		v := 0.0
		if s == current {
			v = 1.0
		}
		c.poolScrubState.WithLabelValues(poolName, s).Set(v)
	}
}

// recordVdevs walks the parsed status tree and emits one sample per leaf
// disk vdev. Aggregate vdevs (mirror, raidz, log/cache groups) carry no
// path so they're skipped — the leaf disks underneath them already
// account for the read/write/cksum totals.
func (c *ZFSCollector) recordVdevs(poolName string, st *pool.Status) {
	if st == nil {
		return
	}
	for _, v := range st.Vdevs {
		c.walkVdev(poolName, v, v.Type)
	}
}

func (c *ZFSCollector) walkVdev(poolName string, v pool.Vdev, parentType string) {
	if v.Type == "disk" && v.Path != "" {
		// "vdev" label is the parent vdev type (mirror-0 → "mirror"); for
		// stripe pools the parent is the disk's own type, which we map to
		// "stripe" so PromQL queries can group disks by topology.
		grouping := parentType
		if grouping == "disk" {
			grouping = "stripe"
		}
		c.vdevReadErr.WithLabelValues(poolName, grouping, v.Path).Set(float64(v.ReadErr))
		c.vdevWriteErr.WithLabelValues(poolName, grouping, v.Path).Set(float64(v.WriteErr))
		c.vdevCksumErr.WithLabelValues(poolName, grouping, v.Path).Set(float64(v.CksumErr))
	}
	for _, child := range v.Children {
		c.walkVdev(poolName, child, v.Type)
	}
}
