package snapshot

import (
	"context"
	"fmt"
	"strings"

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

// --- Holds -----------------------------------------------------------------

// validateHoldTag checks the user-defined hold tag: non-empty, no leading
// dash, alphanumeric/dash/underscore/dot.
func validateHoldTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("hold tag empty")
	}
	if strings.HasPrefix(tag, "-") {
		return fmt.Errorf("hold tag cannot start with '-'")
	}
	for _, r := range tag {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.'
		if !ok {
			return fmt.Errorf("hold tag has illegal character %q", r)
		}
	}
	return nil
}

func buildHoldArgs(snapshot, tag string, recursive bool) ([]string, error) {
	if err := names.ValidateSnapshotName(snapshot); err != nil {
		return nil, err
	}
	if err := validateHoldTag(tag); err != nil {
		return nil, err
	}
	args := []string{"hold"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, tag, snapshot)
	return args, nil
}

func (m *Manager) Hold(ctx context.Context, snapshot, tag string, recursive bool) error {
	args, err := buildHoldArgs(snapshot, tag, recursive)
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

func buildReleaseArgs(snapshot, tag string, recursive bool) ([]string, error) {
	if err := names.ValidateSnapshotName(snapshot); err != nil {
		return nil, err
	}
	if err := validateHoldTag(tag); err != nil {
		return nil, err
	}
	args := []string{"release"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, tag, snapshot)
	return args, nil
}

func (m *Manager) Release(ctx context.Context, snapshot, tag string, recursive bool) error {
	args, err := buildReleaseArgs(snapshot, tag, recursive)
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

func buildHoldsArgs(snapshot string) ([]string, error) {
	if err := names.ValidateSnapshotName(snapshot); err != nil {
		return nil, err
	}
	return []string{"holds", "-H", "-p", snapshot}, nil
}

func (m *Manager) Holds(ctx context.Context, snapshot string) ([]Hold, error) {
	args, err := buildHoldsArgs(snapshot)
	if err != nil {
		return nil, err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	out, err := runner(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseHolds(out)
}
