package snapshot

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/exec"
)

type Manager struct {
	ZFSBin string
}

// List returns snapshots recursively under dataset, or all snapshots on the
// host if dataset is "". dataset may be a pool, filesystem, or volume path.
func (m *Manager) List(ctx context.Context, dataset string) ([]Snapshot, error) {
	args := []string{"list", "-H", "-p", "-t", "snapshot",
		"-o", "name,used,referenced,creation"}
	if dataset != "" {
		args = append(args, "-r", dataset)
	}
	out, err := exec.Run(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseList(out)
}
