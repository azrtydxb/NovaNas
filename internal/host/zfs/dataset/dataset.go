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

func (m *Manager) List(ctx context.Context, pool string) ([]Dataset, error) {
	args := []string{"list", "-H", "-p", "-t", "filesystem,volume",
		"-o", "name,type,used,available,referenced,mountpoint,compression,recordsize"}
	if pool != "" {
		args = append(args, "-r", pool)
	}
	out, err := exec.Run(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseList(out)
}
