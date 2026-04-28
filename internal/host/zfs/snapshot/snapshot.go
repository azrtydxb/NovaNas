package snapshot

import (
	"context"
	"errors"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// ErrNotFound is returned by Get/Destroy/Rollback when the snapshot does not
// exist. List paths do not surface ErrNotFound; an empty result is "no
// matches" rather than "not found".
var ErrNotFound = errors.New("snapshot not found")

type Manager struct {
	ZFSBin string
}

// List returns snapshots recursively under root, or all snapshots on the
// host if root is "". root may be a pool ("tank"), a filesystem path
// ("tank/home"), or a volume ("tank/vol1") — `zfs list -r` recurses below
// any dataset target.
func (m *Manager) List(ctx context.Context, root string) ([]Snapshot, error) {
	args := []string{"list", "-H", "-p", "-t", "snapshot",
		"-o", "name,used,referenced,creation"}
	if root != "" {
		args = append(args, "-r", root)
	}
	out, err := exec.Run(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseList(out)
}
