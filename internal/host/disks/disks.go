package disks

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/exec"
)

type Lister struct {
	LsblkBin string
}

func (l *Lister) List(ctx context.Context) ([]Disk, error) {
	out, err := exec.Run(ctx, l.LsblkBin, "-J", "-b",
		"-o", "NAME,SIZE,MODEL,SERIAL,TYPE,ROTA,FSTYPE,MOUNTPOINT,WWN")
	if err != nil {
		return nil, err
	}
	return parseLsblk(out)
}
