package pool

import (
	"context"
	"errors"
)

// ScrubInfo summarises the current scrub/scan state of a pool. It's a
// subset of *Detail produced by Get and is the surface the scrubpolicy
// executor + metrics collector consume so they don't have to depend on
// the whole pool.Detail tree. CksumErr is the sum across all leaf vdevs.
type ScrubInfo struct {
	// State is one of: "none", "in-progress", "finished", "resilver".
	State        string
	ScannedBytes uint64
	TotalBytes   uint64
	// Errors is the sum of read+write+checksum errors across the pool's
	// leaf vdevs. Useful as a quick post-scrub indicator without parsing
	// the whole status tree again.
	Errors uint64
	// RawLine is the verbatim "scan:" line for diagnostics.
	RawLine string
}

// ScrubStatus returns the current scrub state for the named pool. It
// wraps Get and projects only the scan-relevant fields. Returns
// ErrNotFound (from Get) if the pool doesn't exist.
func (m *Manager) ScrubStatus(ctx context.Context, name string) (*ScrubInfo, error) {
	d, err := m.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	info := &ScrubInfo{State: "none"}
	if d.Status != nil && d.Status.Scan != nil {
		info.State = d.Status.Scan.State
		info.ScannedBytes = d.Status.Scan.ScannedBytes
		info.TotalBytes = d.Status.Scan.TotalBytes
		info.RawLine = d.Status.Scan.RawLine
	}
	if d.Status != nil {
		info.Errors = sumVdevErrors(d.Status.Vdevs)
	}
	return info, nil
}

// IsScrubInProgress reports whether the named pool currently has a scrub
// in progress. Convenience wrapper around ScrubStatus for the executor's
// "skip if already scrubbing" guard.
func (m *Manager) IsScrubInProgress(ctx context.Context, name string) (bool, error) {
	info, err := m.ScrubStatus(ctx, name)
	if err != nil {
		return false, err
	}
	return info.State == "in-progress", nil
}

// IsResilverInProgress reports whether the named pool is currently
// resilvering. ZFS triggers resilvers automatically on disk replacement;
// we observe but don't manage them.
func (m *Manager) IsResilverInProgress(ctx context.Context, name string) (bool, error) {
	info, err := m.ScrubStatus(ctx, name)
	if err != nil {
		return false, err
	}
	return info.State == "resilver", nil
}

// StartScrub is a convenience wrapper for Scrub(ctx, name, ScrubStart).
// Defined so callers don't have to import the ScrubAction enum just to
// pass "start".
func (m *Manager) StartScrub(ctx context.Context, name string) error {
	return m.Scrub(ctx, name, ScrubStart)
}

// StopScrub is a convenience wrapper for Scrub(ctx, name, ScrubStop).
func (m *Manager) StopScrub(ctx context.Context, name string) error {
	return m.Scrub(ctx, name, ScrubStop)
}

// PoolNames returns the names of all pools currently visible to the
// host. Used by the scrub-policy executor to expand a "*" pool selector
// at fire time.
func (m *Manager) PoolNames(ctx context.Context) ([]string, error) {
	pools, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(pools))
	for _, p := range pools {
		out = append(out, p.Name)
	}
	return out, nil
}

// sumVdevErrors walks the parsed vdev tree and sums error counts at leaf
// (disk) vdevs. Aggregate vdevs (mirror, raidz, log/cache groups) carry
// no path so their counters are folded into the leaves underneath.
func sumVdevErrors(vdevs []Vdev) uint64 {
	var total uint64
	for _, v := range vdevs {
		if v.Type == "disk" && v.Path != "" {
			total += v.ReadErr + v.WriteErr + v.CksumErr
		}
		total += sumVdevErrors(v.Children)
	}
	return total
}

// ErrScrubAlreadyRunning is returned by HigherLevelScrub when the caller
// asks for a scrub on a pool whose previous scrub is still running.
var ErrScrubAlreadyRunning = errors.New("scrub already in progress")
