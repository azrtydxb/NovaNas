package iscsi

import (
	"strconv"
	"strings"
)

// targetcli's `ls` output is a tree of indented lines, each beginning
// with a `o- ` marker after some whitespace and optional pipe characters
// drawn for visual continuation. Example for /iscsi ls depth=1:
//
//	o- iscsi ............................................. [Targets: 1]
//	  o- iqn.2020-01.io.example:foo .................. [TPGs: 1]
//
// And for /iscsi/<iqn> ls (full target detail):
//
//	o- iqn.2020-01.io.example:foo ................. [TPGs: 1]
//	  o- tpg1 ......................................... [...]
//	    o- acls ..................................... [ACLs: 1]
//	    | o- iqn.2020-01.io.client:bar ............ [Mapped LUNs: 1]
//	    o- luns ..................................... [LUNs: 1]
//	    | o- lun0 ............... [block/zvol1 (/dev/zvol/tank/vol1)]
//	    o- portals .................................. [Portals: 1]
//	      o- 10.0.0.1:3260 ........................... [OK]
//
// Parser scope:
//   - Recognises only `o- <name>` nodes; skips blanks/headers.
//   - Strips the trailing `... [<info>]` decoration.
//   - Distinguishes children of `acls`, `luns`, `portals` by tracking the
//     most recent section header (case-insensitive match on the bare name).
//   - LUN parsing extracts id from `lunN`, and best-effort pulls
//     backstore + zvol path from the trailing `[block/<name> (<dev>)]`
//     decoration when present.
//   - Portal parsing splits the bare name on the last `:` into host and port.
//
// What is NOT handled:
//   - Multi-TPG layouts (we only consume tpg1; other tpg sections are
//     scanned through but their contents land under the last-seen
//     section header, which may produce noise. v1 only exposes tpg1).
//   - Mapped-LUN sub-entries beneath an ACL are ignored.
//   - CHAP credentials surfaced anywhere in the tree are intentionally
//     never returned (the wrapper blanks them defensively).
//   - The dotted "filler" between name and bracketed info varies by
//     terminal width and version; we only rely on the `o- ` prefix and
//     the indent depth.

// parseTargetList walks `targetcli /iscsi ls depth=1` output.
// Every `o- iqn.*` entry under the top-level `o- iscsi` line is a target.
func parseTargetList(data []byte) ([]Target, error) {
	var out []Target
	for _, line := range strings.Split(string(data), "\n") {
		_, name, ok := parseTreeLine(line)
		if !ok {
			continue
		}
		if strings.HasPrefix(name, "iqn.") {
			out = append(out, Target{IQN: name})
		}
	}
	return out, nil
}

// parseTargetDetail walks `targetcli /iscsi/<iqn> ls` output.
func parseTargetDetail(data []byte) (*TargetDetail, error) {
	d := &TargetDetail{}
	section := "" // "acls" | "luns" | "portals" | ""
	for _, line := range strings.Split(string(data), "\n") {
		_, name, ok := parseTreeLine(line)
		if !ok {
			continue
		}
		// Section headers reset what subsequent leaf entries mean.
		switch strings.ToLower(name) {
		case "acls":
			section = "acls"
			continue
		case "luns":
			section = "luns"
			continue
		case "portals":
			section = "portals"
			continue
		}
		// Skip the wrapping iqn / tpg lines.
		if strings.HasPrefix(name, "iqn.") && section == "" {
			continue
		}
		if strings.HasPrefix(name, "tpg") {
			// Entering a tpg resets section context.
			section = ""
			continue
		}
		switch section {
		case "acls":
			if strings.HasPrefix(name, "iqn.") {
				d.ACLs = append(d.ACLs, ACL{InitiatorIQN: name})
			}
		case "luns":
			if lun, ok := parseLUNLine(name, line); ok {
				d.LUNs = append(d.LUNs, lun)
			}
		case "portals":
			if p, ok := parsePortalName(name); ok {
				d.Portals = append(d.Portals, p)
			}
		}
	}
	return d, nil
}

// parseTreeLine returns the indent-column of the `o-` marker and the
// node name, with the trailing dotted filler / bracketed info stripped.
func parseTreeLine(line string) (indent int, name string, ok bool) {
	idx := strings.Index(line, "o- ")
	if idx < 0 {
		return 0, "", false
	}
	rest := line[idx+3:]
	// Strip trailing " ... [info]" or just trailing dots.
	if dot := strings.Index(rest, " ."); dot >= 0 {
		rest = rest[:dot]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return 0, "", false
	}
	return idx, rest, true
}

// parseLUNLine parses a leaf LUN entry. `name` is the bare node name
// (e.g. "lun0") and `fullLine` is the original line so we can recover
// the bracketed `[block/<backstore> (<dev>)]` annotation.
func parseLUNLine(name, fullLine string) (LUN, bool) {
	if !strings.HasPrefix(name, "lun") {
		return LUN{}, false
	}
	idStr := strings.TrimPrefix(name, "lun")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return LUN{}, false
	}
	lun := LUN{ID: id}
	// Best-effort enrichment from the bracketed annotation.
	if open := strings.LastIndex(fullLine, "["); open >= 0 {
		if close := strings.Index(fullLine[open:], "]"); close > 0 {
			info := fullLine[open+1 : open+close]
			// info looks like: "block/zvol1 (/dev/zvol/tank/vol1)"
			if strings.HasPrefix(info, "block/") {
				rest := info[len("block/"):]
				if sp := strings.Index(rest, " "); sp > 0 {
					lun.Backstore = rest[:sp]
					if p := strings.Index(rest[sp:], "("); p >= 0 {
						zvol := rest[sp+p+1:]
						if e := strings.Index(zvol, ")"); e >= 0 {
							lun.Zvol = zvol[:e]
						}
					}
				} else {
					lun.Backstore = rest
				}
			}
		}
	}
	return lun, true
}

// parsePortalName parses a portal node name of the form "<ip>:<port>".
// IPv6 addresses surface as "[::1]:3260" — we accept that form too by
// splitting on the LAST colon after the closing bracket (if any).
func parsePortalName(name string) (Portal, bool) {
	host, port := name, ""
	if strings.HasPrefix(name, "[") {
		end := strings.Index(name, "]")
		if end < 0 {
			return Portal{}, false
		}
		host = name[1:end]
		rest := name[end+1:]
		if strings.HasPrefix(rest, ":") {
			port = rest[1:]
		}
	} else if i := strings.LastIndex(name, ":"); i >= 0 {
		host = name[:i]
		port = name[i+1:]
	}
	if host == "" || port == "" {
		return Portal{}, false
	}
	pn, err := strconv.Atoi(port)
	if err != nil {
		return Portal{}, false
	}
	return Portal{IP: host, Port: pn, Transport: "tcp"}, true
}
