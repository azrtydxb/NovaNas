// Package snapshot wraps `zfs` for snapshot operations.
package snapshot

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type Snapshot struct {
	Name           string `json:"name"`
	Dataset        string `json:"dataset"`
	ShortName      string `json:"shortName"`
	UsedBytes      uint64 `json:"usedBytes"`
	ReferencedBytes uint64 `json:"referencedBytes"`
	CreationUnix   int64  `json:"creationUnix"`
}

func parseList(data []byte) ([]Snapshot, error) {
	var out []Snapshot
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			return nil, fmt.Errorf("zfs snapshot list: 4 fields expected, got %d in %q", len(f), line)
		}
		s := Snapshot{Name: f[0]}
		// ZFS forbids '@' in dataset and snapshot-shortname components, so
		// the first '@' is always the separator. IndexByte (not
		// LastIndexByte) is intentional. `at <= 0` rejects both "no @" and
		// a leading @ (empty dataset).
		at := strings.IndexByte(f[0], '@')
		if at <= 0 {
			return nil, fmt.Errorf("snapshot name missing '@': %q", f[0])
		}
		s.Dataset = f[0][:at]
		s.ShortName = f[0][at+1:]
		var err error
		if s.UsedBytes, err = parseUint(f[1]); err != nil {
			return nil, err
		}
		if s.ReferencedBytes, err = parseUint(f[2]); err != nil {
			return nil, err
		}
		if s.CreationUnix, err = strconv.ParseInt(f[3], 10, 64); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

func parseUint(s string) (uint64, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}
