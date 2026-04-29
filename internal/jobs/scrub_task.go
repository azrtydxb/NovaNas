package jobs

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

// DispatchScrub is a small helper around Dispatcher.Dispatch for the
// scrub-policy executor and the ad-hoc "/api/v1/pools/{name}/scrub"
// handler. Both call sites would otherwise duplicate the
// payload/uniquekey wiring; centralising it here keeps the
// "scheduled scrub" + "operator-triggered scrub" flows identical.
//
// The handler that runs the task is jobs.handlePoolScrub (worker.go),
// which calls pool.Manager.Scrub(ctx, name, action). MaxRetry=0 (the
// dispatcher default) is appropriate: a failed scrub should surface to
// the operator via the jobs API rather than silently auto-retry, since
// "scrub already running" is a common cause and re-queueing is harmful.
//
// The UniqueKey "pool:<name>:scrub" prevents two concurrent scrub
// dispatches against the same pool from coexisting in the queue. If the
// scheduler tick fires while a previous scheduled scrub is still
// inflight for the same pool, the second Dispatch returns
// jobs.ErrDuplicate and the executor skips + logs.
func DispatchScrub(ctx context.Context, d *Dispatcher, name string, action pool.ScrubAction, requestID, source string) (DispatchOutput, error) {
	cmd := "zpool scrub " + name
	if action == pool.ScrubStop {
		cmd = "zpool scrub -s " + name
	}
	return d.Dispatch(ctx, DispatchInput{
		Kind:      KindPoolScrub,
		Target:    name,
		Payload:   PoolScrubPayload{Name: name, Action: action},
		Command:   cmd,
		RequestID: requestID,
		UniqueKey: "pool:" + name + ":scrub",
	})
}
