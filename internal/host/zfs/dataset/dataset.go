package dataset

import (
	"context"
	"errors"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

var ErrNotFound = errors.New("dataset not found")

type Manager struct {
	ZFSBin string
}

type Detail struct {
	Dataset Dataset           `json:"dataset"`
	Props   map[string]string `json:"properties"`
}

// List returns datasets recursively under root, or all datasets if root is "".
// root may be a pool ("tank") or a dataset path ("tank/home"). For dataset
// roots, only that subtree is returned.
func (m *Manager) List(ctx context.Context, root string) ([]Dataset, error) {
	args := []string{"list", "-H", "-p", "-t", "filesystem,volume",
		"-o", "name,type,used,available,referenced,mountpoint,compression,recordsize"}
	if root != "" {
		args = append(args, "-r", root)
	}
	out, err := exec.Run(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseList(out)
}

// notFoundErr maps ZFS's `cannot open 'X': dataset does not exist` stderr
// (stable across OpenZFS versions) to ErrNotFound. Returns the original
// error otherwise.
//
// TODO(plan-2): once Manager grows a Runner func field for testability,
// add a unit test that drives this path with a stubbed *exec.HostError.
func notFoundErr(err error) error {
	if err == nil {
		return nil
	}
	var he *exec.HostError
	if errors.As(err, &he) && strings.Contains(he.Stderr, "does not exist") {
		return ErrNotFound
	}
	return err
}

// Get returns full detail (the row + all properties) for a single dataset
// by name. Returns ErrNotFound if the dataset does not exist.
func (m *Manager) Get(ctx context.Context, name string) (*Detail, error) {
	listOut, err := exec.Run(ctx, m.ZFSBin, "list", "-H", "-p",
		"-t", "filesystem,volume",
		"-o", "name,type,used,available,referenced,mountpoint,compression,recordsize",
		name)
	if err != nil {
		return nil, notFoundErr(err)
	}
	ds, err := parseList(listOut)
	if err != nil {
		return nil, err
	}
	if len(ds) == 0 {
		return nil, ErrNotFound
	}
	propsOut, err := exec.Run(ctx, m.ZFSBin, "get", "-H", "-p", "all", name)
	if err != nil {
		// Race window: dataset destroyed between list and get; surface
		// ErrNotFound consistently.
		return nil, notFoundErr(err)
	}
	props, err := parseProps(propsOut)
	if err != nil {
		return nil, err
	}
	return &Detail{Dataset: ds[0], Props: props}, nil
}
