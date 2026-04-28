// Package dataset wraps `zfs` for filesystem and volume datasets.
package dataset

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type Dataset struct {
	Name           string `json:"name"`
	Type           string `json:"type"` // filesystem|volume
	UsedBytes      uint64 `json:"usedBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	ReferencedBytes uint64 `json:"referencedBytes"`
	Mountpoint     string `json:"mountpoint,omitempty"`
	Compression    string `json:"compression,omitempty"`
	RecordSizeBytes uint64 `json:"recordSizeBytes,omitempty"`
}

func parseList(data []byte) ([]Dataset, error) {
	var out []Dataset
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 8 {
			return nil, fmt.Errorf("zfs list: 8 fields expected, got %d in %q", len(f), line)
		}
		d := Dataset{
			Name:        f[0],
			Type:        f[1],
			Compression: f[6],
		}
		var err error
		if d.UsedBytes, err = parseUint(f[2]); err != nil {
			return nil, err
		}
		if d.AvailableBytes, err = parseUint(f[3]); err != nil {
			return nil, err
		}
		if d.ReferencedBytes, err = parseUint(f[4]); err != nil {
			return nil, err
		}
		if f[5] != "-" {
			d.Mountpoint = f[5]
		}
		if d.RecordSizeBytes, err = parseUint(f[7]); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, sc.Err()
}

func parseUint(s string) (uint64, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}
