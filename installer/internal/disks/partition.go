package disks

import (
	"fmt"
	"os/exec"
)

// PartitionLayout describes the canonical NovaNas GPT scheme.
//
// Sizes in MiB. Offsets are cumulative; Persistent consumes the remaining
// capacity up to (but not beyond) the 80 GB target, leaving any remainder
// unallocated so it is not accidentally trampled by a future re-install.
type PartitionLayout struct {
	EFIMiB        int64
	BootMiB       int64
	OSAMiB        int64
	OSBMiB        int64
	PersistentMiB int64
}

// DefaultLayout matches docs/06 partition layout.
func DefaultLayout() PartitionLayout {
	return PartitionLayout{
		EFIMiB:        512,
		BootMiB:       2 * 1024,
		OSAMiB:        4 * 1024,
		OSBMiB:        4 * 1024,
		PersistentMiB: 80 * 1024,
	}
}

// PartitionPlan is a rendered plan of parted commands, safe to log and to
// inspect before execution.
type PartitionPlan struct {
	Device   string
	Commands [][]string // each entry is argv for exec.Command
}

// BuildPartitionPlan renders the parted invocation and mkfs calls for a single
// disk. It does NOT execute anything.
func BuildPartitionPlan(device string, l PartitionLayout) PartitionPlan {
	// parted ranges are in MiB from the disk start; 1 MiB lead-in for alignment.
	start := int64(1)
	efiEnd := start + l.EFIMiB
	bootEnd := efiEnd + l.BootMiB
	osAEnd := bootEnd + l.OSAMiB
	osBEnd := osAEnd + l.OSBMiB
	persistentEnd := osBEnd + l.PersistentMiB

	partedArgs := []string{
		"--script", device,
		"mklabel", "gpt",
		"mkpart", "EFI", "fat32", fmt.Sprintf("%dMiB", start), fmt.Sprintf("%dMiB", efiEnd),
		"set", "1", "esp", "on",
		"mkpart", "Boot", "ext4", fmt.Sprintf("%dMiB", efiEnd), fmt.Sprintf("%dMiB", bootEnd),
		"mkpart", "OS-A", "ext4", fmt.Sprintf("%dMiB", bootEnd), fmt.Sprintf("%dMiB", osAEnd),
		"mkpart", "OS-B", "ext4", fmt.Sprintf("%dMiB", osAEnd), fmt.Sprintf("%dMiB", osBEnd),
		"mkpart", "Persistent", "ext4", fmt.Sprintf("%dMiB", osBEnd), fmt.Sprintf("%dMiB", persistentEnd),
	}

	plan := PartitionPlan{Device: device}
	plan.Commands = append(plan.Commands, append([]string{"parted"}, partedArgs...))

	// Format commands; part device names follow the kernel convention:
	// sda -> sda1, nvme0n1 -> nvme0n1p1.
	p1 := PartName(device, 1)
	p2 := PartName(device, 2)
	p3 := PartName(device, 3)
	p4 := PartName(device, 4)
	p5 := PartName(device, 5)

	plan.Commands = append(plan.Commands,
		[]string{"mkfs.vfat", "-F", "32", "-n", "EFI", p1},
		[]string{"mkfs.ext4", "-F", "-L", "boot", p2},
		[]string{"mkfs.ext4", "-F", "-L", "os-a", p3},
		[]string{"mkfs.ext4", "-F", "-L", "os-b", p4},
		[]string{"mkfs.ext4", "-F", "-L", "persistent", p5},
	)
	return plan
}

// PartName returns the Linux partition device name for a given disk + index.
// /dev/sda + 1 => /dev/sda1, /dev/nvme0n1 + 1 => /dev/nvme0n1p1.
func PartName(device string, index int) string {
	last := device[len(device)-1]
	if last >= '0' && last <= '9' {
		return fmt.Sprintf("%sp%d", device, index)
	}
	return fmt.Sprintf("%s%d", device, index)
}

// Runner executes a plan. If DryRun is true, commands are only logged.
type Runner struct {
	DryRun bool
	Exec   func(name string, args ...string) error
	Log    func(format string, args ...any)
}

// NewRunner returns a Runner with sensible defaults.
func NewRunner(dryRun bool, log func(format string, args ...any)) *Runner {
	return &Runner{
		DryRun: dryRun,
		Log:    log,
		Exec: func(name string, args ...string) error {
			cmd := exec.Command(name, args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s: %w: %s", name, err, string(out))
			}
			return nil
		},
	}
}

// Apply executes the plan. Returns on the first error.
func (r *Runner) Apply(p PartitionPlan) error {
	for _, cmd := range p.Commands {
		if r.Log != nil {
			r.Log("exec: %v", cmd)
		}
		if r.DryRun {
			continue
		}
		if err := r.Exec(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}
	return nil
}
