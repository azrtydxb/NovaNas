package rdma

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestList_SingleAdapterTwoPorts(t *testing.T) {
	root := t.TempDir()
	dev := filepath.Join(root, "mlx5_0")

	writeFile(t, filepath.Join(dev, "board_id"), "MT_0000000080\n")
	writeFile(t, filepath.Join(dev, "hca_type"), "MT4119\n")

	// Port 1: ACTIVE Ethernet (RoCE) with GIDs
	writeFile(t, filepath.Join(dev, "ports", "1", "state"), "4: ACTIVE\n")
	writeFile(t, filepath.Join(dev, "ports", "1", "link_layer"), "Ethernet\n")
	writeFile(t, filepath.Join(dev, "ports", "1", "gids", "0"), "0000:0000:0000:0000:0000:0000:0000:0000\n")
	writeFile(t, filepath.Join(dev, "ports", "1", "gids", "1"), "fe80:0000:0000:0000:0202:c9ff:fe00:0001\n")

	// Port 2: DOWN InfiniBand
	writeFile(t, filepath.Join(dev, "ports", "2", "state"), "1: DOWN\n")
	writeFile(t, filepath.Join(dev, "ports", "2", "link_layer"), "InfiniBand\n")

	l := &Lister{SysPath: root}
	got, err := l.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 adapter, got %d", len(got))
	}
	a := got[0]
	if a.Name != "mlx5_0" {
		t.Errorf("Name=%q", a.Name)
	}
	if a.BoardID != "MT_0000000080" {
		t.Errorf("BoardID=%q", a.BoardID)
	}
	if a.HCAType != "MT4119" {
		t.Errorf("HCAType=%q", a.HCAType)
	}
	if len(a.Ports) != 2 {
		t.Fatalf("want 2 ports, got %d", len(a.Ports))
	}
	if a.Ports[0].Number != 1 || a.Ports[0].State != "ACTIVE" || a.Ports[0].LinkLayer != "Ethernet" {
		t.Errorf("port1 = %+v", a.Ports[0])
	}
	if len(a.Ports[0].GIDs) != 1 || a.Ports[0].GIDs[0] != "fe80:0000:0000:0000:0202:c9ff:fe00:0001" {
		t.Errorf("port1 GIDs = %+v (expected first non-zero)", a.Ports[0].GIDs)
	}
	if a.Ports[1].Number != 2 || a.Ports[1].State != "DOWN" || a.Ports[1].LinkLayer != "InfiniBand" {
		t.Errorf("port2 = %+v", a.Ports[1])
	}

	// HasActiveRDMA convenience
	active, err := l.HasActiveRDMA(context.Background())
	if err != nil {
		t.Fatalf("HasActiveRDMA: %v", err)
	}
	if !active {
		t.Errorf("HasActiveRDMA=false, want true")
	}
}

func TestList_MissingRoot(t *testing.T) {
	l := &Lister{SysPath: filepath.Join(t.TempDir(), "does-not-exist")}
	got, err := l.List(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil slice, got %+v", got)
	}

	active, err := l.HasActiveRDMA(context.Background())
	if err != nil {
		t.Fatalf("HasActiveRDMA: %v", err)
	}
	if active {
		t.Errorf("expected no active RDMA on missing root")
	}
}

func TestList_MalformedState(t *testing.T) {
	root := t.TempDir()
	dev := filepath.Join(root, "mlx5_0")
	// state file with no colon — should not crash, state should be ""
	writeFile(t, filepath.Join(dev, "ports", "1", "state"), "garbage-no-colon\n")
	writeFile(t, filepath.Join(dev, "ports", "1", "link_layer"), "Ethernet\n")

	l := &Lister{SysPath: root}
	got, err := l.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || len(got[0].Ports) != 1 {
		t.Fatalf("unexpected adapters: %+v", got)
	}
	p := got[0].Ports[0]
	if p.Number != 1 {
		t.Errorf("Number=%d", p.Number)
	}
	if p.State != "" {
		t.Errorf("expected empty state on malformed file, got %q", p.State)
	}
	if p.LinkLayer != "Ethernet" {
		t.Errorf("LinkLayer=%q", p.LinkLayer)
	}
}
