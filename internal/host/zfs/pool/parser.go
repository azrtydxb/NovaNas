// Package pool wraps `zpool` for read and write operations on storage pools.
package pool

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type Pool struct {
	Name          string `json:"name"`
	SizeBytes     uint64 `json:"sizeBytes"`
	Allocated     uint64 `json:"allocated"`
	Free          uint64 `json:"free"`
	Health        string `json:"health"`
	ReadOnly      bool   `json:"readOnly"`
	Fragmentation int    `json:"fragmentationPct"`
	Capacity      int    `json:"capacityPct"`
	DedupRatio    string `json:"dedupRatio"`
}

func parseList(data []byte) ([]Pool, error) {
	var out []Pool
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 9 {
			return nil, fmt.Errorf("zpool list: expected 9 fields, got %d in %q", len(f), line)
		}
		p := Pool{
			Name:       f[0],
			Health:     f[4],
			ReadOnly:   f[5] == "on",
			DedupRatio: f[8],
		}
		var err error
		if p.SizeBytes, err = parseUint(f[1]); err != nil {
			return nil, err
		}
		if p.Allocated, err = parseUint(f[2]); err != nil {
			return nil, err
		}
		if p.Free, err = parseUint(f[3]); err != nil {
			return nil, err
		}
		if p.Fragmentation, err = parseInt(f[6]); err != nil {
			return nil, err
		}
		if p.Capacity, err = parseInt(f[7]); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, sc.Err()
}

// ScrubStatus holds the parsed state of the most recent pool scan/scrub.
type ScrubStatus struct {
	// State is one of: "none", "in-progress", "finished", "resilver".
	State        string `json:"state"`
	RawLine      string `json:"rawLine,omitempty"`
	ScannedBytes uint64 `json:"scannedBytes,omitempty"`
	TotalBytes   uint64 `json:"totalBytes,omitempty"`
}

type Status struct {
	State string       `json:"state"`
	Scan  *ScrubStatus `json:"scan,omitempty"`
	Vdevs []Vdev       `json:"vdevs"`
}

type Vdev struct {
	Type     string `json:"type"`     // mirror, raidz1, raidz2, raidz3, stripe, log, cache, spare
	Path     string `json:"path,omitempty"`
	State    string `json:"state"`
	ReadErr  uint64 `json:"readErrors"`
	WriteErr uint64 `json:"writeErrors"`
	CksumErr uint64 `json:"checksumErrors"`
	Children []Vdev `json:"children,omitempty"`
}

func parseProps(data []byte) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			return nil, fmt.Errorf("zpool get: bad line %q", line)
		}
		// f[0]=pool name, f[1]=property, f[2]=value, f[3]=source
		out[f[1]] = f[2]
	}
	return out, sc.Err()
}

// parseStatus parses `zpool status -P` output into a Status with a
// 2-level vdev tree. ZFS output is irregular:
//   - Pool root: 1 leading tab
//   - Mirror/raidz/draid groups: 1 tab + 2 spaces (one indent step)
//   - Group-leaf disks: 1 tab + 4 spaces (two indent steps)
//   - logs/cache/spares group HEADERS: 1 tab (same as pool root)
//   - logs/cache/spares LEAVES: 1 tab + 2 spaces (same indent as mirror)
//
// Different visual indent for semantic siblings, so depth-stack tracking
// is unreliable. Instead: classify each row by name pattern. Known
// vdev-group types become top-level entries; everything else becomes
// either a child of the most recent group or a top-level leaf (for
// striped pools whose top level is bare disks).
func parseStatus(data []byte) (*Status, error) {
	st := &Status{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	inConfig := false
	rootSeen := false

	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "state:") {
			st.State = strings.TrimSpace(strings.TrimPrefix(trim, "state:"))
			continue
		}
		if strings.HasPrefix(trim, "scan:") {
			st.Scan = parseScrubStatus(strings.TrimSpace(strings.TrimPrefix(trim, "scan:")))
			continue
		}
		if strings.HasPrefix(trim, "config:") {
			inConfig = true
			continue
		}
		if strings.HasPrefix(trim, "errors:") {
			inConfig = false
			continue
		}
		if !inConfig || trim == "" || strings.HasPrefix(trim, "NAME") {
			continue
		}
		fields := strings.Fields(trim)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		state := "-"
		if len(fields) >= 2 {
			state = fields[1]
		}
		if !rootSeen {
			rootSeen = true
			continue
		}
		v := classifyVdev(name, state)

		// If this row is a top-level vdev group, append it.
		// If it's a leaf and the most recent top-level is a group, nest it.
		// Otherwise (no vdevs yet, or last top-level is itself a leaf — a
		// stripe-of-disks layout), append as another top-level leaf.
		if isVdevGroup(v.Type) {
			st.Vdevs = append(st.Vdevs, v)
		} else if n := len(st.Vdevs); n > 0 && isVdevGroup(st.Vdevs[n-1].Type) {
			st.Vdevs[n-1].Children = append(st.Vdevs[n-1].Children, v)
		} else {
			st.Vdevs = append(st.Vdevs, v)
		}
	}
	return st, sc.Err()
}

func classifyVdev(name, state string) Vdev {
	v := Vdev{State: state}
	switch {
	case strings.HasPrefix(name, "mirror"):
		v.Type = "mirror"
	case strings.HasPrefix(name, "raidz3"):
		v.Type = "raidz3"
	case strings.HasPrefix(name, "raidz2"):
		v.Type = "raidz2"
	case strings.HasPrefix(name, "raidz1"), strings.HasPrefix(name, "raidz"):
		v.Type = "raidz1"
	case strings.HasPrefix(name, "draid"):
		v.Type = "draid"
	case name == "logs":
		v.Type = "log"
	case name == "cache":
		v.Type = "cache"
	case name == "spares":
		v.Type = "spare"
	default:
		v.Type = "disk"
		v.Path = name
	}
	return v
}

func isVdevGroup(t string) bool {
	switch t {
	case "mirror", "raidz1", "raidz2", "raidz3", "draid", "log", "cache", "spare":
		return true
	}
	return false
}

func parseUint(s string) (uint64, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}

func parseInt(s string) (int, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

// parseScrubStatus interprets the content after "scan: " from zpool status.
// raw is that content, already stripped of the "scan:" prefix and trimmed.
// On any unexpected format it falls back to State="finished" with just RawLine.
func parseScrubStatus(raw string) *ScrubStatus {
	ss := &ScrubStatus{RawLine: raw}
	switch {
	case raw == "none requested":
		ss.State = "none"
	case strings.HasPrefix(raw, "scrub in progress"):
		ss.State = "in-progress"
		// "... 1.50G scanned at ..., 0B issued at ..., 5.00G total ..."
		if scanned, ok := extractToken(raw, "scanned"); ok {
			ss.ScannedBytes = parseHumanSize(scanned)
		}
		if total, ok := extractToken(raw, "total"); ok {
			ss.TotalBytes = parseHumanSize(total)
		}
	case strings.HasPrefix(raw, "resilvered"):
		ss.State = "resilver"
	case strings.HasPrefix(raw, "scrub repaired"):
		ss.State = "finished"
	default:
		ss.State = "finished"
	}
	return ss
}

// extractToken finds the token immediately before the given keyword word in s.
// e.g. extractToken("1.50G scanned at 1.00G/s, 5.00G total", "scanned") → "1.50G", true
func extractToken(s, keyword string) (string, bool) {
	idx := strings.Index(s, " "+keyword)
	if idx < 0 {
		return "", false
	}
	before := strings.TrimSpace(s[:idx])
	// grab the last whitespace-separated token
	parts := strings.Fields(before)
	if len(parts) == 0 {
		return "", false
	}
	return parts[len(parts)-1], true
}

// parseHumanSize converts a human-readable size string (e.g. "1.50G", "750M",
// "5G", "0B") to bytes. Unrecognised formats return 0.
func parseHumanSize(s string) uint64 {
	if len(s) == 0 {
		return 0
	}
	suffixes := map[byte]uint64{
		'B': 1,
		'K': 1024,
		'M': 1024 * 1024,
		'G': 1024 * 1024 * 1024,
		'T': 1024 * 1024 * 1024 * 1024,
		'P': 1024 * 1024 * 1024 * 1024 * 1024,
	}
	last := s[len(s)-1]
	mult, ok := suffixes[last]
	if !ok {
		// Try pure integer
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	num := s[:len(s)-1]
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	return uint64(f * float64(mult))
}
