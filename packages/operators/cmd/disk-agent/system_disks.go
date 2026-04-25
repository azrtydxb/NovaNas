package main

// System-disk detection: any block device whose partition is currently
// mounted by the host (or carries swap, or is part of an md/lvm/dmcrypt
// device that backs a host mount) is the OS disk and must NEVER be
// offered for pool assignment.
//
// We read /proc/self/mounts (host /proc is mounted into the agent
// pod via the DaemonSet template) and walk every "source" that
// starts with /dev/ back to its parent block device. The result is a
// set of disk names like {"nvme0n1", "sda"}; isSystemDisk() returns
// true if its argument is in the set.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// partitionSuffixRE matches the partition number trailing a base
// device name. Linux conventions:
//   nvme0n1 → partitions: nvme0n1p1, nvme0n1p2 …    (suffix "p<N>")
//   sda     → partitions: sda1, sda2 …              (suffix "<N>")
//   mmcblk0 → partitions: mmcblk0p1, …              (suffix "p<N>")
// Stripping these gives the base disk.
var partitionSuffixRE = regexp.MustCompile(`(p?\d+)$`)

// loadMountedDevices returns the set of base block-device names that
// have at least one mounted partition (or are mounted directly).
//
// We read multiple sources because /proc/mounts inside a container is
// often the container's own mountns; the DaemonSet mounts the host's
// /proc at /host/proc so we look there first.
func loadMountedDevices() map[string]struct{} {
	out := map[string]struct{}{}
	for _, path := range []string{"/host/proc/mounts", "/proc/mounts", "/proc/self/mounts"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for s.Scan() {
			fields := strings.Fields(s.Text())
			if len(fields) < 2 {
				continue
			}
			src := fields[0]
			if !strings.HasPrefix(src, "/dev/") {
				continue
			}
			devName := strings.TrimPrefix(src, "/dev/")
			if base := baseDiskName(devName); base != "" {
				out[base] = struct{}{}
			}
		}
		_ = f.Close()
	}
	// Also consider swap entries (/proc/swaps) — swap partitions
	// rarely show up in /proc/mounts.
	for _, path := range []string{"/host/proc/swaps", "/proc/swaps"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		s := bufio.NewScanner(f)
		first := true
		for s.Scan() {
			if first { // skip header
				first = false
				continue
			}
			fields := strings.Fields(s.Text())
			if len(fields) < 1 {
				continue
			}
			src := fields[0]
			if !strings.HasPrefix(src, "/dev/") {
				continue
			}
			devName := strings.TrimPrefix(src, "/dev/")
			if base := baseDiskName(devName); base != "" {
				out[base] = struct{}{}
			}
		}
		_ = f.Close()
	}
	return out
}

// baseDiskName walks a partition name (or device-mapper slave) up to
// its parent disk. Examples:
//   nvme0n1p3 → nvme0n1
//   sda2      → sda
//   dm-0      → resolved via /sys/block/dm-0/slaves/* (then recursed)
//   mapper/x  → resolved via /sys/block/dm-0/slaves/* if applicable
func baseDiskName(devName string) string {
	devName = strings.TrimPrefix(devName, "mapper/")
	// device-mapper: the dm-N node has slave devices; pick the first
	// real backing block device and recurse.
	if strings.HasPrefix(devName, "dm-") || strings.HasPrefix(devName, "md") {
		entries, err := os.ReadDir(filepath.Join("/sys/block", devName, "slaves"))
		if err != nil {
			return ""
		}
		for _, e := range entries {
			if base := baseDiskName(e.Name()); base != "" {
				return base
			}
		}
		return ""
	}
	// Plain block device or partition. /sys/block/<base>/<partition>
	// is the canonical layout; the base directory exists if devName is
	// itself a disk (no further work needed).
	if _, err := os.Stat(filepath.Join("/sys/block", devName)); err == nil {
		return devName
	}
	// Strip the partition suffix and try again.
	base := partitionSuffixRE.ReplaceAllString(devName, "")
	if base != devName {
		if _, err := os.Stat(filepath.Join("/sys/block", base)); err == nil {
			return base
		}
	}
	return ""
}

// isSystemDisk returns (true, reason) if any partition of devName (or
// devName itself) is in the set of mounted/swap-backing devices.
func isSystemDisk(devName string, mounted map[string]struct{}) (bool, string) {
	if _, ok := mounted[devName]; ok {
		return true, fmt.Sprintf("device %s is mounted by host", devName)
	}
	// Walk partitions and check each.
	entries, err := os.ReadDir(filepath.Join("/sys/block", devName))
	if err != nil {
		return false, ""
	}
	for _, e := range entries {
		// Partition entries have a sub-directory named like the disk
		// with a digit suffix.
		if !strings.HasPrefix(e.Name(), devName) {
			continue
		}
		// /proc/mounts uses the partition's own short name (e.g.
		// nvme0n1p1), and we collected those — but we walked them
		// to their parent already. Re-check here in case the host
		// mounted the disk directly without partitioning.
		if _, ok := mounted[e.Name()]; ok {
			return true, fmt.Sprintf("partition /dev/%s is mounted", e.Name())
		}
	}
	return false, ""
}
