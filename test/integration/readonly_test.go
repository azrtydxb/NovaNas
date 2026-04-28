//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/novanas/nova-nas/internal/api"
	"github.com/novanas/nova-nas/internal/host/disks"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/store"
)

type stubDisks struct{}

func (stubDisks) List(_ context.Context) ([]disks.Disk, error) {
	return []disks.Disk{{Name: "sda", SizeBytes: 1000}}, nil
}

type stubPools struct{}

func (stubPools) List(_ context.Context) ([]pool.Pool, error) {
	return []pool.Pool{{Name: "tank", Health: "ONLINE"}}, nil
}
func (stubPools) Get(_ context.Context, name string) (*pool.Detail, error) {
	if name == "tank" {
		return &pool.Detail{Pool: pool.Pool{Name: "tank"}}, nil
	}
	return nil, pool.ErrNotFound
}

type stubDatasets struct{}

func (stubDatasets) List(_ context.Context, _ string) ([]dataset.Dataset, error) {
	return []dataset.Dataset{{Name: "tank/home", Type: "filesystem"}}, nil
}
func (stubDatasets) Get(_ context.Context, name string) (*dataset.Detail, error) {
	if name == "tank/home" {
		return &dataset.Detail{Dataset: dataset.Dataset{Name: "tank/home"}}, nil
	}
	return nil, dataset.ErrNotFound
}

type stubSnapshots struct{}

func (stubSnapshots) List(_ context.Context, _ string) ([]snapshot.Snapshot, error) {
	return []snapshot.Snapshot{{Name: "tank/home@a", Dataset: "tank/home", ShortName: "a"}}, nil
}

func TestReadOnlyEndpoints(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv := api.New(api.Deps{
		Logger:    logger,
		Store:     st,
		Disks:     stubDisks{},
		Pools:     stubPools{},
		Datasets:  stubDatasets{},
		Snapshots: stubSnapshots{},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		path string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/api/v1/disks", http.StatusOK},
		{"/api/v1/pools", http.StatusOK},
		{"/api/v1/pools/tank", http.StatusOK},
		{"/api/v1/pools/missing", http.StatusNotFound},
		{"/api/v1/datasets", http.StatusOK},
		{"/api/v1/datasets/" + url.PathEscape("tank/home"), http.StatusOK},
		{"/api/v1/datasets/" + url.PathEscape("missing"), http.StatusNotFound},
		{"/api/v1/snapshots", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + c.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != c.want {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("status=%d want=%d body=%s", resp.StatusCode, c.want, body)
			}
		})
	}

	// Sanity check JSON shape on /pools
	resp, _ := http.Get(ts.URL + "/api/v1/pools")
	var pools []pool.Pool
	_ = json.NewDecoder(resp.Body).Decode(&pools)
	if len(pools) != 1 || pools[0].Name != "tank" {
		t.Errorf("pools=%+v", pools)
	}
}
