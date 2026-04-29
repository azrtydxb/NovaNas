package csi

import (
	"errors"
	"strings"
)

// VolumeID is the opaque CSI VolumeID encoded as the dataset's full ZFS name,
// i.e. "<pool>/<...parent>/<leaf>". The pool is the first segment; the leaf is
// the final segment; the parent is whatever lies between.
//
// Example: VolumeID "tank/csi/pvc-abc" → pool="tank", parent="tank/csi",
// leaf="pvc-abc".
//
// We intentionally do NOT encode the volume mode (filesystem vs zvol) in the
// ID. The dataset's `type` ZFS property is the source of truth and is fetched
// via GetDataset when needed.

// VolumeID is a parsed CSI volume identifier.
type VolumeID struct {
	Full   string // tank/csi/pvc-abc
	Pool   string // tank
	Parent string // tank/csi
	Leaf   string // pvc-abc
}

// EncodeVolumeID returns the canonical VolumeID string for parent + leaf.
func EncodeVolumeID(parent, leaf string) string {
	if parent == "" {
		return leaf
	}
	return parent + "/" + leaf
}

// ParseVolumeID parses a VolumeID string.
func ParseVolumeID(id string) (VolumeID, error) {
	if id == "" {
		return VolumeID{}, errors.New("empty volume id")
	}
	parts := strings.Split(id, "/")
	if len(parts) < 2 {
		return VolumeID{}, errors.New("volume id must contain at least pool/name")
	}
	for _, p := range parts {
		if p == "" {
			return VolumeID{}, errors.New("volume id has empty path segment")
		}
	}
	return VolumeID{
		Full:   id,
		Pool:   parts[0],
		Parent: strings.Join(parts[:len(parts)-1], "/"),
		Leaf:   parts[len(parts)-1],
	}, nil
}

// SnapshotID is encoded as "<dataset-full>@<short>".
type SnapshotID struct {
	Full     string
	Dataset  string
	ShortTag string
}

// EncodeSnapshotID joins dataset and short name.
func EncodeSnapshotID(dataset, short string) string {
	return dataset + "@" + short
}

// ParseSnapshotID parses a snapshot identifier.
func ParseSnapshotID(id string) (SnapshotID, error) {
	at := strings.LastIndex(id, "@")
	if at <= 0 || at == len(id)-1 {
		return SnapshotID{}, errors.New("snapshot id must be <dataset>@<name>")
	}
	return SnapshotID{Full: id, Dataset: id[:at], ShortTag: id[at+1:]}, nil
}

// ZvolDevicePath returns the kernel device path for a zvol VolumeID.
func ZvolDevicePath(id VolumeID) string {
	// /dev/zvol/<pool>/<parent-after-pool>/<leaf>
	rest := strings.TrimPrefix(id.Full, "")
	return "/dev/zvol/" + rest
}
