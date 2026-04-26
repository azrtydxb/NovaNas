package main

// Inline copy of the disk-discovery logic from
// storage/internal/disk/discovery.go. We can't import that path
// directly from another Go module (it's marked `internal/`), and
// moving it to a public package is a separate refactor.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type deviceType int

const (
	typeUnknown deviceType = iota
	typeNVMe
	typeSSD
	typeHDD
)

type deviceInfo struct {
	Path       string
	SizeBytes  uint64
	DeviceType deviceType
	Model      string
	Serial     string
	Wwn        string
	ByIdPath   string
	Rotational bool
	// System disks host the OS — any partition on them is currently
	// mounted by the host (root, /boot, /boot/efi, swap, k3s state,
	// etc). Disk CRs are still created so admins can see the hardware,
	// but they get the novanas.io/system label and the pool-attach UI
	// filters them out.
	System     bool
	// SystemReason gives a human-readable explanation when System is
	// true (e.g. "partition nvme0n1p1 mounted at /boot/efi").
	SystemReason string
}

func (d deviceType) String() string {
	switch d {
	case typeNVMe:
		return "nvme"
	case typeSSD:
		return "ssd"
	case typeHDD:
		return "hdd"
	default:
		return ""
	}
}

func discoverDevices() ([]deviceInfo, error) {
	const sysBlock = "/sys/block"
	entries, err := os.ReadDir(sysBlock)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sysBlock, err)
	}
	mounts := loadMountedDevices()
	var devices []deviceInfo
	for _, entry := range entries {
		name := entry.Name()
		// Skip kernel virtual / pseudo block devices that are never real
		// storage: loop, ram, dm-*, sr* (cdrom), zd* (zfs), md* (mdraid
		// container — its members surface separately), zram, fd*, and
		// nbd* (network block device — used by SPDK to expose virtual
		// LUNs to the host; these have size 0 until something is
		// attached and would otherwise pollute the disk inventory).
		switch {
		case strings.HasPrefix(name, "loop"),
			strings.HasPrefix(name, "ram"),
			strings.HasPrefix(name, "dm-"),
			strings.HasPrefix(name, "sr"),
			strings.HasPrefix(name, "zd"),
			strings.HasPrefix(name, "zram"),
			strings.HasPrefix(name, "fd"),
			strings.HasPrefix(name, "nbd"),
			strings.HasPrefix(name, "md"):
			continue
		}
		dev := deviceInfo{Path: filepath.Join("/dev", name)}
		dev.System, dev.SystemReason = isSystemDisk(name, mounts)
		if data, err := os.ReadFile(filepath.Join(sysBlock, name, "size")); err == nil {
			var sectors uint64
			fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &sectors)
			dev.SizeBytes = sectors * 512
		}
		// Reject zero-sized devices outright. Real drives always report
		// non-zero block counts; size 0 here means a kernel placeholder
		// (empty card-reader slot, detached nbd, etc).
		if dev.SizeBytes == 0 {
			continue
		}
		if strings.HasPrefix(name, "nvme") {
			dev.DeviceType = typeNVMe
		} else {
			if data, err := os.ReadFile(filepath.Join(sysBlock, name, "queue", "rotational")); err == nil {
				if strings.TrimSpace(string(data)) == "1" {
					dev.DeviceType = typeHDD
					dev.Rotational = true
				} else {
					dev.DeviceType = typeSSD
				}
			}
		}
		if data, err := os.ReadFile(filepath.Join(sysBlock, name, "device", "model")); err == nil {
			dev.Model = strings.TrimSpace(string(data))
		}
		for _, p := range []string{
			filepath.Join(sysBlock, name, "device", "serial"),
		} {
			if data, err := os.ReadFile(p); err == nil {
				if s := strings.TrimSpace(string(data)); s != "" {
					dev.Serial = s
					break
				}
			}
		}
		for _, p := range []string{
			filepath.Join(sysBlock, name, "device", "wwid"),
			filepath.Join(sysBlock, name, "wwid"),
		} {
			if data, err := os.ReadFile(p); err == nil {
				if w := strings.TrimSpace(string(data)); w != "" {
					dev.Wwn = w
					break
				}
			}
		}
		dev.ByIdPath = lookupByIdPath(name)
		devices = append(devices, dev)
	}
	return devices, nil
}

// lookupByIdPath returns the first /dev/disk/by-id/* entry whose
// readlink target points at the given device.
func lookupByIdPath(devName string) string {
	const dir = "/dev/disk/by-id"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	want := "/" + devName
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		target, err := os.Readlink(full)
		if err != nil {
			continue
		}
		if strings.HasSuffix(target, want) {
			return full
		}
	}
	return ""
}
