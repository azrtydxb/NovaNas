package main

import (
	"context"

	"github.com/azrtydxb/novanas/storage/internal/controller"
)

// poolPollerRunnable adapts controller.PoolPoller to the
// manager.Runnable interface so it starts and stops with the
// controller-runtime manager.
type poolPollerRunnable struct {
	poller *controller.PoolPoller
}

func (r *poolPollerRunnable) Start(ctx context.Context) error {
	return r.poller.Run(ctx)
}
