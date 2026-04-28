package snapshot

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

func buildCreateArgs(dataset, short string, recursive bool) ([]string, error) {
	full := dataset + "@" + short
	if err := names.ValidateSnapshotName(full); err != nil {
		return nil, err
	}
	args := []string{"snapshot"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, full)
	return args, nil
}

func (m *Manager) Create(ctx context.Context, dataset, short string, recursive bool) error {
	args, err := buildCreateArgs(dataset, short, recursive)
	if err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err = runner(ctx, m.ZFSBin, args...)
	return err
}

func (m *Manager) Destroy(ctx context.Context, name string) error {
	if err := names.ValidateSnapshotName(name); err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err := runner(ctx, m.ZFSBin, "destroy", name)
	return err
}

func (m *Manager) Rollback(ctx context.Context, snapshot string) error {
	if err := names.ValidateSnapshotName(snapshot); err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err := runner(ctx, m.ZFSBin, "rollback", "-r", snapshot)
	return err
}
