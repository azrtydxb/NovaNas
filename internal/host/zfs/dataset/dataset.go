package dataset

import (
	"context"
	"errors"

	"github.com/novanas/nova-nas/internal/host/exec"
)

var ErrNotFound = errors.New("dataset not found")

type Manager struct {
	ZFSBin string
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
