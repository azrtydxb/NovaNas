package pool

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

// TrimAction selects start vs stop for Manager.Trim.
type TrimAction string

const (
	TrimStart TrimAction = "start"
	TrimStop  TrimAction = "stop"
)

// Trim issues `zpool trim` (start) or `zpool trim -c` (stop) against a
// pool, optionally scoped to a single device. If disk is empty, the
// whole pool is trimmed.
func (m *Manager) Trim(ctx context.Context, name string, action TrimAction, disk string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	if action != TrimStart && action != TrimStop {
		return fmt.Errorf("invalid trim action %q", action)
	}
	args := []string{"trim"}
	if action == TrimStop {
		args = append(args, "-c")
	}
	args = append(args, name)
	if disk != "" {
		if strings.HasPrefix(disk, "-") {
			return fmt.Errorf("disk path cannot start with '-'")
		}
		args = append(args, disk)
	}
	_, err := m.run(ctx, args...)
	return err
}

// SetProps sets one or more pool properties. Each property runs as its
// own `zpool set k=v <name>` invocation, mirroring dataset.SetProps.
// Keys are sorted to give deterministic ordering for tests.
func (m *Manager) SetProps(ctx context.Context, name string, props map[string]string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		if k == "" || strings.HasPrefix(k, "-") {
			return fmt.Errorf("invalid property name %q", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := m.run(ctx, "set", k+"="+props[k], name); err != nil {
			return err
		}
	}
	return nil
}

// validWaitActivities mirrors the activity tokens accepted by
// `zpool wait -t`.
var validWaitActivities = map[string]struct{}{
	"discard":    {},
	"free":       {},
	"initialize": {},
	"replace":    {},
	"remove":     {},
	"resilver":   {},
	"scrub":      {},
	"trim":       {},
}

// Wait blocks until the named pool activity completes, or until the
// supplied timeout elapses (timeout <= 0 means no extra deadline).
func (m *Manager) Wait(ctx context.Context, name string, activity string, timeout time.Duration) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	if _, ok := validWaitActivities[activity]; !ok {
		return fmt.Errorf("invalid wait activity %q", activity)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	_, err := m.run(ctx, "wait", "-t", activity, name)
	return err
}
