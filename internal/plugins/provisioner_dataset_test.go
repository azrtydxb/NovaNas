package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

type fakeDatasetClient struct {
	exists      map[string]bool
	createCalls []dataset.CreateSpec
	destroyed   []string
}

func newFakeDatasetClient() *fakeDatasetClient {
	return &fakeDatasetClient{exists: map[string]bool{}}
}

func (f *fakeDatasetClient) Get(_ context.Context, name string) (*dataset.Detail, error) {
	if f.exists[name] {
		return &dataset.Detail{}, nil
	}
	return nil, dataset.ErrNotFound
}

func (f *fakeDatasetClient) Create(_ context.Context, spec dataset.CreateSpec) error {
	f.createCalls = append(f.createCalls, spec)
	f.exists[spec.Parent+"/"+spec.Name] = true
	return nil
}

func (f *fakeDatasetClient) Destroy(_ context.Context, name string, _ bool) error {
	f.destroyed = append(f.destroyed, name)
	delete(f.exists, name)
	return nil
}

func TestDatasetProvisioner_CreateAndDestroy(t *testing.T) {
	c := newFakeDatasetClient()
	p := &DatasetProvisioner{Client: c}
	id, err := p.Provision(context.Background(), "rustfs", DatasetNeed{
		Pool: "tank", Name: "rustfs/data",
		Properties: map[string]string{"compression": "zstd"},
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if id != "dataset:rustfs/tank/rustfs/data" {
		t.Errorf("id=%q", id)
	}
	if len(c.createCalls) != 1 {
		t.Fatalf("create calls=%d", len(c.createCalls))
	}
	if c.createCalls[0].Type != "filesystem" {
		t.Errorf("type=%q", c.createCalls[0].Type)
	}

	if err := p.Unprovision(context.Background(), "rustfs", id); err != nil {
		t.Fatalf("unprovision: %v", err)
	}
	if len(c.destroyed) != 1 || c.destroyed[0] != "tank/rustfs/data" {
		t.Errorf("destroyed=%v", c.destroyed)
	}
}

func TestDatasetProvisioner_Idempotent(t *testing.T) {
	c := newFakeDatasetClient()
	c.exists["tank/x"] = true
	p := &DatasetProvisioner{Client: c}
	id, err := p.Provision(context.Background(), "p", DatasetNeed{Pool: "tank", Name: "x"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if len(c.createCalls) != 0 {
		t.Errorf("expected no create on existing dataset, got %d", len(c.createCalls))
	}
	if id != "dataset:p/tank/x" {
		t.Errorf("id=%q", id)
	}
}

func TestDatasetProvisioner_UnprovisionMissing(t *testing.T) {
	c := newFakeDatasetClient()
	p := &DatasetProvisioner{Client: c}
	// Tolerate already-gone.
	if err := p.Unprovision(context.Background(), "p", "dataset:p/tank/missing"); err != nil {
		t.Fatalf("unprovision missing: %v", err)
	}
	if len(c.destroyed) != 0 {
		t.Errorf("destroyed=%v", c.destroyed)
	}
}

func TestDatasetProvisioner_BadResourceID(t *testing.T) {
	p := &DatasetProvisioner{Client: newFakeDatasetClient()}
	if err := p.Unprovision(context.Background(), "p", "wrong:id"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDatasetProvisioner_GetError(t *testing.T) {
	c := &errDatasetClient{}
	p := &DatasetProvisioner{Client: c}
	if _, err := p.Provision(context.Background(), "p", DatasetNeed{Pool: "t", Name: "n"}); err == nil {
		t.Fatal("expected error")
	}
}

type errDatasetClient struct{}

func (errDatasetClient) Get(_ context.Context, _ string) (*dataset.Detail, error) {
	return nil, errors.New("boom")
}
func (errDatasetClient) Create(_ context.Context, _ dataset.CreateSpec) error {
	return errors.New("nope")
}
func (errDatasetClient) Destroy(_ context.Context, _ string, _ bool) error {
	return errors.New("nope")
}
