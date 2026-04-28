package pool

import (
	"context"
	"errors"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

var ErrNotFound = errors.New("pool not found")

type Manager struct {
	ZpoolBin string
	// Runner overrides exec.Run for tests. nil → use exec.Run directly.
	Runner exec.Runner
}

type Detail struct {
	Pool   Pool              `json:"pool"`
	Props  map[string]string `json:"properties"`
	Status *Status           `json:"status"`
}

func (m *Manager) Get(ctx context.Context, name string) (*Detail, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	listOut, err := runner(ctx, m.ZpoolBin, "list", "-H", "-p",
		"-o", "name,size,allocated,free,health,readonly,fragmentation,capacity,dedupratio", name)
	if err != nil {
		var he *exec.HostError
		if errors.As(err, &he) && strings.Contains(he.Stderr, "no such pool") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	pools, err := parseList(listOut)
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return nil, ErrNotFound
	}

	propsOut, err := runner(ctx, m.ZpoolBin, "get", "-H", "-p", "all", name)
	if err != nil {
		return nil, err
	}
	props, err := parseProps(propsOut)
	if err != nil {
		return nil, err
	}

	statusOut, err := runner(ctx, m.ZpoolBin, "status", "-P", name)
	if err != nil {
		return nil, err
	}
	st, err := parseStatus(statusOut)
	if err != nil {
		return nil, err
	}

	return &Detail{Pool: pools[0], Props: props, Status: st}, nil
}

func (m *Manager) List(ctx context.Context) ([]Pool, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	out, err := runner(ctx, m.ZpoolBin, "list", "-H", "-p",
		"-o", "name,size,allocated,free,health,readonly,fragmentation,capacity,dedupratio")
	if err != nil {
		return nil, err
	}
	return parseList(out)
}
