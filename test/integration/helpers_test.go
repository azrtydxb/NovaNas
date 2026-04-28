//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/store"
)

// stubExec returns a no-op exec runner that always succeeds.
func stubExec(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

// startTestServer spins up a full chi+asynq+worker stack against the
// shared Postgres (dbDSN) and Redis (redisURL) containers from
// main_test.go. Returns a httptest.Server pointing at the chi handler.
func startTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	asyncOpt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	asyncClient := asynq.NewClient(asyncOpt)
	t.Cleanup(func() { _ = asyncClient.Close() })

	rcOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rc := redis.NewClient(rcOpts)
	t.Cleanup(func() { _ = rc.Close() })

	pm := &pool.Manager{ZpoolBin: "/bin/true", Runner: stubExec}
	dm := &dataset.Manager{ZFSBin: "/bin/true", Runner: stubExec}
	sm := &snapshot.Manager{ZFSBin: "/bin/true", Runner: stubExec}

	disp := &jobs.Dispatcher{Client: asyncClient, Queries: st.Queries, Pool: st.Pool}

	asyncSrv := asynq.NewServer(asyncOpt, asynq.Config{Concurrency: 2})
	mux := jobs.NewServeMux(jobs.WorkerDeps{
		Logger: logger, Queries: st.Queries, Redis: rc,
		Pools: pm, Datasets: dm, Snapshots: sm,
	})
	go func() { _ = asyncSrv.Run(mux) }()
	t.Cleanup(asyncSrv.Stop)

	srv := api.New(api.Deps{
		Logger:     logger,
		Store:      st,
		Disks:      stubDisks{},
		Pools:      pm,
		Datasets:   dm,
		Snapshots:  sm,
		Dispatcher: disp,
		Redis:      rc,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}
