package disks

import "fmt"

// MirrorPlan renders the mdadm commands to create a RAID-1 mirror across two
// boot disks. The mirror is created at the block-device level (not per-
// partition) so the partition layout is identical to the single-disk case but
// lives on /dev/md0.
type MirrorPlan struct {
	MDDevice string
	Commands [][]string
}

// BuildMirrorPlan returns a mdadm create + zero-superblock plan.
func BuildMirrorPlan(mdDevice string, disks []string) (MirrorPlan, error) {
	if len(disks) != 2 {
		return MirrorPlan{}, fmt.Errorf("mirror requires exactly 2 disks, got %d", len(disks))
	}
	plan := MirrorPlan{MDDevice: mdDevice}
	// Wipe any prior superblocks.
	for _, d := range disks {
		plan.Commands = append(plan.Commands, []string{"mdadm", "--zero-superblock", "--force", d})
	}
	// Create the mirror.
	create := []string{
		"mdadm", "--create", mdDevice,
		"--level=1",
		"--raid-devices=2",
		"--metadata=1.2",
		"--run",
	}
	create = append(create, disks...)
	plan.Commands = append(plan.Commands, create)
	return plan, nil
}
