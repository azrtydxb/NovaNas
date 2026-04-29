package csi

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Mounter abstracts the small set of host operations the Node service needs.
// The real implementation shells out to /bin/mount, /bin/umount, blkid,
// mkfs.ext4 / mkfs.xfs, resize2fs, xfs_growfs. Tests provide a fake.
type Mounter interface {
	BindMount(source, target string, readonly bool) error
	Unmount(target string) error
	IsMounted(target string) (bool, error)
	Mkfs(device, fsType string) error
	IsFormatted(device string) (bool, string, error) // (yes, fstype)
	GrowFS(target, device, fsType string) error
	EnsureDir(path string) error
	EnsureFile(path string) error
}

// shellMounter is the production Mounter. It assumes a Linux host with the
// usual util-linux + e2fsprogs/xfsprogs binaries on PATH.
type shellMounter struct{}

// NewShellMounter returns the default Mounter.
func NewShellMounter() Mounter { return &shellMounter{} }

func (shellMounter) EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func (shellMounter) EnsureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(parentDir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

func parentDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

func (shellMounter) BindMount(source, target string, readonly bool) error {
	args := []string{"--bind", source, target}
	if err := run("mount", args...); err != nil {
		return err
	}
	if readonly {
		// Remount readonly after the bind is established.
		return run("mount", "-o", "remount,ro,bind", target)
	}
	return nil
}

func (shellMounter) Unmount(target string) error {
	// -l is preferred to avoid blocking on busy mounts during pod cleanup.
	if err := run("umount", target); err != nil {
		// Idempotent: succeed if not mounted.
		if strings.Contains(err.Error(), "not mounted") || strings.Contains(err.Error(), "not currently mounted") {
			return nil
		}
		return err
	}
	return nil
}

func (shellMounter) IsMounted(target string) (bool, error) {
	out, err := exec.Command("findmnt", "-n", "-o", "TARGET", target).CombinedOutput()
	if err != nil {
		// findmnt exits non-zero when the path isn't mounted.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("findmnt: %w: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (shellMounter) Mkfs(device, fsType string) error {
	switch fsType {
	case "ext4":
		return run("mkfs.ext4", "-F", device)
	case "xfs":
		return run("mkfs.xfs", "-f", device)
	default:
		return fmt.Errorf("unsupported fsType %q", fsType)
	}
}

func (shellMounter) IsFormatted(device string) (bool, string, error) {
	out, err := exec.Command("blkid", "-o", "value", "-s", "TYPE", device).CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			// blkid exits 2 when no signature is found.
			return false, "", nil
		}
		return false, "", fmt.Errorf("blkid: %w: %s", err, string(out))
	}
	t := strings.TrimSpace(string(out))
	return t != "", t, nil
}

func (shellMounter) GrowFS(target, device, fsType string) error {
	switch fsType {
	case "ext4":
		return run("resize2fs", device)
	case "xfs":
		return run("xfs_growfs", target)
	default:
		return fmt.Errorf("unsupported fsType %q", fsType)
	}
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(out.String()))
	}
	return nil
}
