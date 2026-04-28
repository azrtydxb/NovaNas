package pool

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/exec"
)

type Manager struct {
	ZpoolBin string
}

func (m *Manager) List(ctx context.Context) ([]Pool, error) {
	out, err := exec.Run(ctx, m.ZpoolBin, "list", "-H", "-p",
		"-o", "name,size,allocated,free,health,readonly,fragmentation,capacity,dedupratio")
	if err != nil {
		return nil, err
	}
	return parseList(out)
}
