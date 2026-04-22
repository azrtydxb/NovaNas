// Package disks scans, partitions, and (optionally) RAID-mirrors the boot disks.
package disks

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// MinDiskBytes is the minimum acceptable OS disk size (16 GB).
const MinDiskBytes int64 = 16 * 1024 * 1024 * 1024

// Disk describes a single physical disk candidate for OS install.
type Disk struct {
	Name      string `json:"name"`     // e.g. "sda"
	Path      string `json:"-"`        // e.g. "/dev/sda"
	SizeBytes int64  `json:"size"`     // bytes
	Model     string `json:"model"`
	Serial    string `json:"serial"`
	Type      string `json:"type"`     // "disk", "part", ...
	Rotational bool  `json:"rota"`
	Transport string `json:"tran"`     // "sata", "nvme", "usb", ...
	Removable bool   `json:"rm"`
}

// lsblkOutput mirrors `lsblk -J` top-level shape.
type lsblkOutput struct {
	Blockdevices []rawDisk `json:"blockdevices"`
}

type rawDisk struct {
	Name       string `json:"name"`
	Size       any    `json:"size"` // int or string depending on lsblk version
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	Type       string `json:"type"`
	Rotational any    `json:"rota"` // bool or "0"/"1"
	Transport  string `json:"tran"`
	Removable  any    `json:"rm"`
}

// Scanner runs lsblk and returns a filtered candidate list.
type Scanner struct {
	// Exec lets tests inject a fake command runner.
	Exec func(name string, args ...string) ([]byte, error)
}

// NewScanner returns a scanner that shells out via exec.Command.
func NewScanner() *Scanner {
	return &Scanner{
		Exec: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).Output()
		},
	}
}

// Scan returns the list of disks suitable for OS install.
func (s *Scanner) Scan() ([]Disk, error) {
	out, err := s.Exec("lsblk", "-J", "-b", "-o", "NAME,SIZE,MODEL,SERIAL,TYPE,ROTA,TRAN,RM")
	if err != nil {
		return nil, fmt.Errorf("lsblk: %w", err)
	}
	return ParseLsblk(out)
}

// ParseLsblk parses the lsblk JSON output and returns OS-install candidates.
// Exposed so tests can feed fixtures.
func ParseLsblk(data []byte) ([]Disk, error) {
	var parsed lsblkOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse lsblk json: %w", err)
	}
	out := make([]Disk, 0, len(parsed.Blockdevices))
	for _, r := range parsed.Blockdevices {
		d := Disk{
			Name:       r.Name,
			Path:       "/dev/" + r.Name,
			Model:      strings.TrimSpace(r.Model),
			Serial:     strings.TrimSpace(r.Serial),
			Type:       r.Type,
			Transport:  r.Transport,
			SizeBytes:  toInt64(r.Size),
			Rotational: toBool(r.Rotational),
			Removable:  toBool(r.Removable),
		}
		if !isCandidate(d) {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func isCandidate(d Disk) bool {
	if d.Type != "disk" {
		return false
	}
	if d.Removable {
		return false
	}
	if d.SizeBytes < MinDiskBytes {
		return false
	}
	if strings.HasPrefix(d.Name, "loop") ||
		strings.HasPrefix(d.Name, "ram") ||
		strings.HasPrefix(d.Name, "zram") ||
		strings.HasPrefix(d.Name, "sr") {
		return false
	}
	return true
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		// lsblk sometimes emits strings; parse best-effort
		var n int64
		fmt.Sscanf(x, "%d", &n)
		return n
	}
	return 0
}

func toBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "1" || strings.EqualFold(x, "true")
	case float64:
		return x != 0
	}
	return false
}

// HumanSize returns a short human-readable representation, e.g. "931.5 GB".
func HumanSize(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"kB", "MB", "GB", "TB", "PB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}
