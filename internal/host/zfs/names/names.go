// Package names validates ZFS pool, dataset, and snapshot names.
package names

import (
	"fmt"
	str "strings"
)

const maxNameLen = 255

var reservedPoolNames = map[string]struct{}{
	"mirror": {}, "raidz": {}, "raidz1": {}, "raidz2": {}, "raidz3": {},
	"draid": {}, "spare": {}, "log": {}, "cache": {}, "special": {},
}

func ValidatePoolName(s string) error {
	if s == "" {
		return fmt.Errorf("pool name empty")
	}
	if len(s) > maxNameLen {
		return fmt.Errorf("pool name too long")
	}
	if _, bad := reservedPoolNames[s]; bad {
		return fmt.Errorf("pool name %q is reserved", s)
	}
	if !isAlpha(rune(s[0])) {
		return fmt.Errorf("pool name must start with a letter")
	}
	for _, r := range s {
		if !isAllowedComponent(r) {
			return fmt.Errorf("pool name has illegal character %q", r)
		}
	}
	return nil
}

func ValidateDatasetName(s string) error {
	if s == "" {
		return fmt.Errorf("dataset name empty")
	}
	if len(s) > maxNameLen {
		return fmt.Errorf("dataset name too long")
	}
	if str.Contains(s, "@") {
		return fmt.Errorf("dataset name cannot contain '@'")
	}
	parts := str.Split(s, "/")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("dataset name must start with pool")
	}
	if err := ValidatePoolName(parts[0]); err != nil {
		return fmt.Errorf("invalid pool component: %w", err)
	}
	for _, p := range parts[1:] {
		if p == "" {
			return fmt.Errorf("dataset name has empty component")
		}
		if str.HasPrefix(p, "-") {
			return fmt.Errorf("dataset component cannot start with '-'")
		}
		for _, r := range p {
			if !isAllowedComponent(r) {
				return fmt.Errorf("dataset component has illegal character %q", r)
			}
		}
	}
	return nil
}

func ValidateSnapshotName(s string) error {
	at := str.IndexByte(s, '@')
	if at <= 0 {
		return fmt.Errorf("snapshot name must contain '<dataset>@<short>'")
	}
	if str.Count(s, "@") != 1 {
		return fmt.Errorf("snapshot name must contain exactly one '@'")
	}
	if err := ValidateDatasetName(s[:at]); err != nil {
		return err
	}
	short := s[at+1:]
	if short == "" {
		return fmt.Errorf("snapshot short name empty")
	}
	for _, r := range short {
		if !isAllowedComponent(r) {
			return fmt.Errorf("snapshot short name has illegal character %q", r)
		}
	}
	return nil
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isAllowedComponent(r rune) bool {
	return isAlpha(r) || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':'
}
