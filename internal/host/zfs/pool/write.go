package pool

import (
	"context"
	"fmt"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

type CreateSpec struct {
	Name  string     `json:"name"`
	Vdevs []VdevSpec `json:"vdevs"`
	Log   []string   `json:"log,omitempty"`
	Cache []string   `json:"cache,omitempty"`
	Spare []string   `json:"spare,omitempty"`
}

type VdevSpec struct {
	Type  string   `json:"type"` // mirror|raidz1|raidz2|raidz3|stripe
	Disks []string `json:"disks"`
}

var validVdevTypes = map[string]struct{}{
	"mirror": {}, "raidz1": {}, "raidz2": {}, "raidz3": {}, "stripe": {},
}

func buildCreateArgs(spec CreateSpec) ([]string, error) {
	if err := names.ValidatePoolName(spec.Name); err != nil {
		return nil, err
	}
	if len(spec.Vdevs) == 0 {
		return nil, fmt.Errorf("pool create: at least one vdev required")
	}
	args := []string{"create", "-f", spec.Name}
	for _, v := range spec.Vdevs {
		if _, ok := validVdevTypes[v.Type]; !ok {
			return nil, fmt.Errorf("invalid vdev type %q", v.Type)
		}
		if len(v.Disks) == 0 {
			return nil, fmt.Errorf("vdev %q has no disks", v.Type)
		}
		if v.Type != "stripe" {
			args = append(args, v.Type)
		}
		args = append(args, v.Disks...)
	}
	if len(spec.Log) > 0 {
		args = append(args, "log")
		args = append(args, spec.Log...)
	}
	if len(spec.Cache) > 0 {
		args = append(args, "cache")
		args = append(args, spec.Cache...)
	}
	if len(spec.Spare) > 0 {
		args = append(args, "spare")
		args = append(args, spec.Spare...)
	}
	return args, nil
}

func (m *Manager) Create(ctx context.Context, spec CreateSpec) error {
	args, err := buildCreateArgs(spec)
	if err != nil {
		return err
	}
	_, err = exec.Run(ctx, m.ZpoolBin, args...)
	return err
}

func (m *Manager) Destroy(ctx context.Context, name string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	_, err := exec.Run(ctx, m.ZpoolBin, "destroy", "-f", name)
	return err
}

type ScrubAction string

const (
	ScrubStart ScrubAction = "start"
	ScrubStop  ScrubAction = "stop"
)

func (m *Manager) Scrub(ctx context.Context, name string, action ScrubAction) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	args := []string{"scrub"}
	if action == ScrubStop {
		args = append(args, "-s")
	}
	args = append(args, name)
	_, err := exec.Run(ctx, m.ZpoolBin, args...)
	return err
}
