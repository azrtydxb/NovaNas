package install

import (
	"fmt"
	"os"
	"os/exec"
)

// SquashfsExtractor clones the live ISO's root squashfs onto a mounted
// slot. Replaces the earlier flow of carrying a second rootfs copy in
// the ISO as a RAUC bundle: the live boot already has the authoritative
// rootfs mounted via live-boot, so the install step just needs to
// replay it onto the target partition.
//
// Candidate squashfs paths, in order of preference:
//  1. /run/live/medium/live/filesystem.squashfs — canonical live-boot
//     media mountpoint.
//  2. /lib/live/mount/medium/live/filesystem.squashfs — older live-boot.
//  3. /cdrom/live/filesystem.squashfs — some bootloader paths.
type SquashfsExtractor struct {
	// DryRun short-circuits the actual extraction (tests + dry-run mode).
	DryRun bool
	// Log is called with each command executed.
	Log func(msg string, kv ...any)
	// Source overrides the auto-detected squashfs path (tests).
	Source string
}

// Candidate live-boot media paths probed by Locate.
var squashfsCandidates = []string{
	"/run/live/medium/live/filesystem.squashfs",
	"/lib/live/mount/medium/live/filesystem.squashfs",
	"/cdrom/live/filesystem.squashfs",
	"/media/cdrom/live/filesystem.squashfs",
}

// Locate returns the first existing squashfs candidate, or an empty
// string when none are present.
func (s *SquashfsExtractor) Locate() string {
	if s.Source != "" {
		return s.Source
	}
	for _, p := range squashfsCandidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return p
		}
	}
	return ""
}

// Extract unsquashfs's the live rootfs into mountpoint. The caller is
// responsible for having formatted + mounted the target partition at
// mountpoint before calling.
func (s *SquashfsExtractor) Extract(mountpoint string) error {
	source := s.Locate()
	if source == "" {
		return fmt.Errorf("squashfs: live rootfs not found in any of %v", squashfsCandidates)
	}
	if s.Log != nil {
		s.Log("unsquashfs", "source", source, "dest", mountpoint)
	}
	if s.DryRun {
		return nil
	}
	// -f: overwrite if dest populated (the caller just mkfs'd, should be empty).
	// -d: destination.
	// -no-progress: quiet on non-tty; progress bars go nowhere useful in our log.
	cmd := exec.Command("unsquashfs", "-f", "-d", mountpoint, "-no-progress", source)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unsquashfs %s -> %s: %w", source, mountpoint, err)
	}
	return nil
}
