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

type Status struct {
	State string `json:"state"`
	Vdevs []Vdev `json:"vdevs"`
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

func parseStatus(data []byte) (*Status, error) {
	st := &Status{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	inConfig := false
	var stack []*Vdev
	rootSeen := false

	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "state:") {
			st.State = strings.TrimSpace(strings.TrimPrefix(trim, "state:"))
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
		if !inConfig || trim == "" {
			continue
		}
		// header line
		if strings.HasPrefix(trim, "NAME") {
			continue
		}
		// Determine indent depth in tabs
		depth := 0
		for _, c := range line {
			if c == '\t' {
				depth++
			} else {
				break
			}
		}
		fields := strings.Fields(trim)
		if len(fields) < 2 {
			continue
		}
		name, state := fields[0], fields[1]

		// Skip the root pool line itself
		if !rootSeen {
			rootSeen = true
			continue
		}

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

		// Pop stack to current depth
		for len(stack) > 0 && len(stack) >= depth {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			st.Vdevs = append(st.Vdevs, v)
			stack = append(stack, &st.Vdevs[len(st.Vdevs)-1])
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, v)
			stack = append(stack, &parent.Children[len(parent.Children)-1])
		}
	}
	return st, sc.Err()
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
