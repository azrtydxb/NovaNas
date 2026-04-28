package pool

import (
	"context"
	"fmt"

	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

// buildCheckpointArgs builds argv for `zpool checkpoint [-d] <name>`.
func buildCheckpointArgs(name string, discard bool) ([]string, error) {
	if err := names.ValidatePoolName(name); err != nil {
		return nil, err
	}
	args := []string{"checkpoint"}
	if discard {
		args = append(args, "-d")
	}
	args = append(args, name)
	return args, nil
}

// Checkpoint creates a pool-wide checkpoint that can be rolled back to.
func (m *Manager) Checkpoint(ctx context.Context, name string) error {
	args, err := buildCheckpointArgs(name, false)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// DiscardCheckpoint discards a previously-created pool checkpoint.
func (m *Manager) DiscardCheckpoint(ctx context.Context, name string) error {
	args, err := buildCheckpointArgs(name, true)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// buildUpgradeArgs builds argv for `zpool upgrade <name>` or `zpool upgrade -a`.
// When all is true, name is ignored. When all is false, name is validated.
func buildUpgradeArgs(name string, all bool) ([]string, error) {
	if all {
		return []string{"upgrade", "-a"}, nil
	}
	if err := names.ValidatePoolName(name); err != nil {
		return nil, err
	}
	return []string{"upgrade", name}, nil
}

// Upgrade upgrades on-disk format of the named pool, or all pools when
// all=true (in which case name is ignored).
func (m *Manager) Upgrade(ctx context.Context, name string, all bool) error {
	args, err := buildUpgradeArgs(name, all)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// buildReguidArgs builds argv for `zpool reguid <name>`.
func buildReguidArgs(name string) ([]string, error) {
	if err := names.ValidatePoolName(name); err != nil {
		return nil, err
	}
	return []string{"reguid", name}, nil
}

// Reguid generates a new unique identifier for the pool.
func (m *Manager) Reguid(ctx context.Context, name string) error {
	args, err := buildReguidArgs(name)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// buildSyncArgs builds argv for `zpool sync [<name>...]`. Empty list means
// sync all pools.
func buildSyncArgs(poolNames []string) ([]string, error) {
	args := []string{"sync"}
	for _, n := range poolNames {
		if err := names.ValidatePoolName(n); err != nil {
			return nil, fmt.Errorf("sync: %w", err)
		}
		args = append(args, n)
	}
	return args, nil
}

// Sync forces in-flight transaction groups to disk for the named pools.
// Empty list syncs all pools.
func (m *Manager) Sync(ctx context.Context, poolNames []string) error {
	args, err := buildSyncArgs(poolNames)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}
