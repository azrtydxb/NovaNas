// Package disks lists physical block devices and their identity.
package disks

import (
	"encoding/json"
	"strconv"
)

type Disk struct {
	Name        string `json:"name"`
	SizeBytes   uint64 `json:"sizeBytes"`
	Model       string `json:"model,omitempty"`
	Serial      string `json:"serial,omitempty"`
	WWN         string `json:"wwn,omitempty"`
	Rotational  bool   `json:"rotational"`
	InUseByPool bool   `json:"inUseByPool"`
}

type lsblkRoot struct {
	BlockDevices []lsblkDev `json:"blockdevices"`
}

type lsblkDev struct {
	Name     string     `json:"name"`
	Size     any        `json:"size"`
	Model    *string    `json:"model"`
	Serial   *string    `json:"serial"`
	Type     string     `json:"type"`
	Rota     bool       `json:"rota"`
	FsType   *string    `json:"fstype"`
	WWN      *string    `json:"wwn"`
	Children []lsblkDev `json:"children"`
}

func parseLsblk(data []byte) ([]Disk, error) {
	var r lsblkRoot
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	out := make([]Disk, 0, len(r.BlockDevices))
	for _, d := range r.BlockDevices {
		if d.Type != "disk" {
			continue
		}
		out = append(out, Disk{
			Name:        d.Name,
			SizeBytes:   parseSize(d.Size),
			Model:       deref(d.Model),
			Serial:      deref(d.Serial),
			WWN:         deref(d.WWN),
			Rotational:  d.Rota,
			InUseByPool: hasZFSMember(d),
		})
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseSize accepts either a JSON number (older lsblk) or a JSON string
// (newer lsblk -b) and returns 0 for unparseable input. The 0 sentinel is
// intentional: a single bad size shouldn't fail the whole disk-list call.
func parseSize(v any) uint64 {
	switch x := v.(type) {
	case float64:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case string:
		n, _ := strconv.ParseUint(x, 10, 64)
		return n
	}
	return 0
}

func hasZFSMember(d lsblkDev) bool {
	if d.FsType != nil && *d.FsType == "zfs_member" {
		return true
	}
	for _, c := range d.Children {
		if hasZFSMember(c) {
			return true
		}
	}
	return false
}
