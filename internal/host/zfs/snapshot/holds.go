package snapshot

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Hold is one row from `zfs holds -H -p <snapshot>`. Each line is
// tab-separated: <snapshot>\t<tag>\t<timestamp>.
type Hold struct {
	Snapshot     string `json:"snapshot"`
	Tag          string `json:"tag"`
	CreationUnix int64  `json:"creationUnix"`
}

func parseHolds(data []byte) ([]Hold, error) {
	var out []Hold
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("zfs holds: 3 fields expected, got %d in %q", len(f), line)
		}
		ts, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("zfs holds: bad timestamp %q: %w", f[2], err)
		}
		out = append(out, Hold{Snapshot: f[0], Tag: f[1], CreationUnix: ts})
	}
	return out, sc.Err()
}
