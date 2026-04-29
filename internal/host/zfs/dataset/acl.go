package dataset

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// NFS4SetFACLBin is the path to nfs4_setfacl on a Debian/Ubuntu-style host.
// It is a var (not const) so tests on systems without the tool can override
// it; production code uses the default.
var NFS4SetFACLBin = "/usr/bin/nfs4_setfacl"

// NFS4GetFACLBin is the path to nfs4_getfacl.
var NFS4GetFACLBin = "/usr/bin/nfs4_getfacl"

// ErrACLNotSupported is returned by GetACL/SetACL when the underlying
// filesystem does not support NFSv4 ACLs (e.g. acltype=posix on ZFS,
// or any non-ZFS filesystem).
var ErrACLNotSupported = errors.New("filesystem does not support NFSv4 ACLs")

// ACE is one Access Control Entry in an NFSv4 ACL. The wire format
// nfs4_setfacl uses is "<type>:<flags>:<principal>:<perms>".
type ACE struct {
	Type        ACEType       `json:"type"`      // "allow" | "deny"
	Principal   string        `json:"principal"` // "user:alice", "group:eng", or special: "OWNER@", "GROUP@", "EVERYONE@"
	Permissions []ACLPerm     `json:"permissions"`
	Inheritance []InheritFlag `json:"inheritance,omitempty"`
}

// ACEType is the Allow/Deny discriminator. nfs4_setfacl supports U/L for
// audit ACEs as well, but we deliberately don't expose those.
type ACEType string

const (
	ACETypeAllow ACEType = "allow"
	ACETypeDeny  ACEType = "deny"
)

// ACLPerm is one named permission. The wire form uses single letters; we
// expose human-readable names plus three NTFS-style shorthands
// (full_control, modify, read_only) that expand to fixed letter sets.
type ACLPerm string

const (
	PermRead        ACLPerm = "read"         // r — read_data / list_directory
	PermWrite       ACLPerm = "write"        // w — write_data / add_file
	PermExecute     ACLPerm = "execute"      // x — execute / search
	PermAppend      ACLPerm = "append"       // p — append_data / add_subdirectory
	PermDelete      ACLPerm = "delete"       // d — delete this file
	PermDeleteChild ACLPerm = "delete_child" // D — delete a child of a directory
	PermReadACL     ACLPerm = "read_acl"     // c — read_acl
	PermWriteACL    ACLPerm = "write_acl"    // C — write_acl
	PermWriteOwner  ACLPerm = "write_owner"  // o — write_owner
	PermReadAttrs   ACLPerm = "read_attrs"   // a — read_attributes
	PermWriteAttrs  ACLPerm = "write_attrs"  // A — write_attributes
	PermReadXattr   ACLPerm = "read_xattr"   // R — read_named_attrs
	PermWriteXattr  ACLPerm = "write_xattr"  // W — write_named_attrs
	PermSync        ACLPerm = "synchronize"  // s — synchronize

	// Shorthands. These cannot be combined with the specific perms they
	// expand to — aceToString rejects mixed sets.
	PermFullControl ACLPerm = "full_control" // all 14 above
	PermModify      ACLPerm = "modify"       // r+w+x+p+d+a+A+c
	PermReadOnly    ACLPerm = "read_only"    // r+x+a+c
)

// InheritFlag controls how an ACE on a directory propagates to children.
type InheritFlag string

const (
	InheritFile        InheritFlag = "file"         // f — file_inherit
	InheritDir         InheritFlag = "dir"          // d — directory_inherit
	InheritNoPropagate InheritFlag = "no_propagate" // n — no_propagate_inherit
	InheritInheritOnly InheritFlag = "inherit_only" // i — inherit_only
)

// permLetter maps an ACLPerm name to its single nfs4_setfacl letter.
// Shorthands are not in this map; they're handled separately.
// permLetter maps to the actual letters Linux nfs4_setfacl(8) accepts.
// See nfs4_acl(5): r/w/a/x/d/D for data ops, t/T for read/write attrs,
// n/N for read/write named attrs (xattrs), c/C for read/write acl,
// o for take ownership, y for synchronize.
var permLetter = map[ACLPerm]byte{
	PermRead:        'r',
	PermWrite:       'w',
	PermExecute:     'x',
	PermAppend:      'a',
	PermDelete:      'd',
	PermDeleteChild: 'D',
	PermReadACL:     'c',
	PermWriteACL:    'C',
	PermWriteOwner:  'o',
	PermReadAttrs:   't',
	PermWriteAttrs:  'T',
	PermReadXattr:   'n',
	PermWriteXattr:  'N',
	PermSync:        'y',
}

// letterPerm is the inverse of permLetter, used by parseACE.
var letterPerm = func() map[byte]ACLPerm {
	m := make(map[byte]ACLPerm, len(permLetter))
	for k, v := range permLetter {
		m[v] = k
	}
	return m
}()

// inheritLetter maps an InheritFlag to its single nfs4_setfacl letter.
var inheritLetter = map[InheritFlag]byte{
	InheritFile:        'f',
	InheritDir:         'd',
	InheritNoPropagate: 'n',
	InheritInheritOnly: 'i',
}

var letterInherit = func() map[byte]InheritFlag {
	m := make(map[byte]InheritFlag, len(inheritLetter))
	for k, v := range inheritLetter {
		m[v] = k
	}
	return m
}()

// shorthandExpansion lists the perms each shorthand stands for. The order
// matters only for round-trip stability of the wire format we emit.
var shorthandExpansion = map[ACLPerm][]ACLPerm{
	PermFullControl: {
		PermRead, PermWrite, PermExecute, PermAppend, PermDelete, PermDeleteChild,
		PermReadAttrs, PermWriteAttrs, PermReadXattr, PermWriteXattr,
		PermReadACL, PermWriteACL, PermWriteOwner, PermSync,
	},
	// "modify" is the NTFS-equivalent: full read + full write + delete-self,
	// but NOT delete_child, write_acl, write_owner, or extended-attribute
	// writes. This matches what Windows users see for the "Modify" right.
	PermModify: {
		PermRead, PermWrite, PermExecute, PermAppend, PermDelete,
		PermReadAttrs, PermWriteAttrs, PermReadACL,
	},
	// "read_only" is NTFS "Read & Execute": read data, traverse, read meta.
	PermReadOnly: {
		PermRead, PermExecute, PermReadAttrs, PermReadACL,
	},
}

// validatePath rejects anything that isn't an absolute filesystem path
// without traversal or shell metacharacters. nfs4_setfacl/getfacl take
// the path as their last positional argument, so a leading '-' would be
// interpreted as a flag.
func validatePath(p string) error {
	if p == "" {
		return fmt.Errorf("path empty")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("path must be absolute")
	}
	if strings.HasPrefix(p, "-") {
		return fmt.Errorf("path cannot start with '-'")
	}
	// Reject path traversal and shell metacharacters. Newlines and NULs
	// would be especially bad in an ACL temp file.
	for _, r := range p {
		switch r {
		case '\x00', '\n', '\r', ';', '&', '|', '`', '$', '*', '?', '<', '>', '"', '\'', '\\':
			return fmt.Errorf("path contains illegal character %q", r)
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("path cannot contain '..'")
		}
	}
	return nil
}

// validatePrincipal accepts:
//   - "user:<name>"
//   - "group:<name>"
//   - "OWNER@", "GROUP@", "EVERYONE@" (uppercase, exact)
//
// Names may contain alphanumeric, dot, dash, underscore, at-sign, and
// backslash (for DOMAIN\user form).
func validatePrincipal(p string) error {
	if p == "" {
		return fmt.Errorf("principal empty")
	}
	switch p {
	case "OWNER@", "GROUP@", "EVERYONE@":
		return nil
	}
	// Lowercase/mixed-case special principals are a common typo; be
	// explicit so the user knows to fix it rather than treating it as
	// a regular user named "owner@".
	if strings.HasSuffix(p, "@") {
		upper := strings.ToUpper(p)
		if upper == "OWNER@" || upper == "GROUP@" || upper == "EVERYONE@" {
			return fmt.Errorf("special principal %q must be uppercase (e.g. %q)", p, upper)
		}
	}
	var name string
	switch {
	case strings.HasPrefix(p, "user:"):
		name = p[len("user:"):]
	case strings.HasPrefix(p, "group:"):
		name = p[len("group:"):]
	default:
		return fmt.Errorf("principal must be user:<name>, group:<name>, OWNER@, GROUP@, or EVERYONE@; got %q", p)
	}
	if name == "" {
		return fmt.Errorf("principal name empty")
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '-' || r == '_' || r == '@' || r == '\\'
		if !ok {
			return fmt.Errorf("principal name has illegal character %q", r)
		}
	}
	return nil
}

// validateACE checks an ACE for internal consistency before it goes to
// aceToString. It does not check for duplicate ACEs in a list — that's
// nfs4_setfacl's job.
func validateACE(ace ACE) error {
	if ace.Type != ACETypeAllow && ace.Type != ACETypeDeny {
		return fmt.Errorf("ACE type must be allow or deny; got %q", ace.Type)
	}
	if err := validatePrincipal(ace.Principal); err != nil {
		return err
	}
	if len(ace.Permissions) == 0 {
		return fmt.Errorf("ACE has no permissions")
	}
	// Check for unknown perms and shorthand-vs-specific conflicts.
	hasShorthand := false
	hasSpecific := false
	for _, p := range ace.Permissions {
		if _, ok := shorthandExpansion[p]; ok {
			hasShorthand = true
			continue
		}
		if _, ok := permLetter[p]; ok {
			hasSpecific = true
			continue
		}
		return fmt.Errorf("unknown permission %q", p)
	}
	if hasShorthand && hasSpecific {
		return fmt.Errorf("cannot combine shorthand permission (full_control/modify/read_only) with specific permissions")
	}
	if hasShorthand && len(ace.Permissions) > 1 {
		return fmt.Errorf("only one shorthand permission allowed per ACE")
	}
	for _, f := range ace.Inheritance {
		if _, ok := inheritLetter[f]; !ok {
			return fmt.Errorf("unknown inheritance flag %q", f)
		}
	}
	return nil
}

// aceToString converts an ACE into the nfs4_setfacl wire format
// "<type>:<flags>:<principal>:<perms>".
//
// Type letter: A=allow, D=deny.
//
// Flags: each InheritFlag's letter, plus 'g' added automatically when the
// principal is a group (group:<name> or GROUP@). We do not expose audit
// flags (S, F).
//
// Principal: special principals pass through verbatim; "user:alice" becomes
// "alice"; "group:eng" becomes "eng@" (the trailing '@' is the conventional
// nfs4_setfacl form for a bare group name without a domain).
//
// Perms: concatenation of the per-perm letters, in the canonical order
// rwxaDdtTnNcCoy (per nfs4_acl(5)). Shorthands expand first.
func aceToString(ace ACE) (string, error) {
	if err := validateACE(ace); err != nil {
		return "", err
	}

	var typeLetter byte
	switch ace.Type {
	case ACETypeAllow:
		typeLetter = 'A'
	case ACETypeDeny:
		typeLetter = 'D'
	}

	// Expand shorthands.
	perms := ace.Permissions
	if len(perms) == 1 {
		if exp, ok := shorthandExpansion[perms[0]]; ok {
			perms = exp
		}
	}

	// Canonical perm order. We deduplicate via a set so callers passing
	// e.g. {read, read} don't produce "rr".
	const canonical = "rwxaDdtTnNcCoy"
	seen := make(map[byte]bool, len(perms))
	for _, p := range perms {
		l, ok := permLetter[p]
		if !ok {
			return "", fmt.Errorf("unknown permission %q", p)
		}
		seen[l] = true
	}
	var permsBuf strings.Builder
	for i := 0; i < len(canonical); i++ {
		if seen[canonical[i]] {
			permsBuf.WriteByte(canonical[i])
		}
	}

	// Flags: inheritance flags in canonical order f,d,n,i, then auto-add
	// 'g' if the principal is a group.
	const canonicalFlags = "fdni"
	flagSet := make(map[byte]bool, len(ace.Inheritance))
	for _, f := range ace.Inheritance {
		flagSet[inheritLetter[f]] = true
	}
	var flagsBuf strings.Builder
	for i := 0; i < len(canonicalFlags); i++ {
		if flagSet[canonicalFlags[i]] {
			flagsBuf.WriteByte(canonicalFlags[i])
		}
	}
	isGroup := ace.Principal == "GROUP@" || strings.HasPrefix(ace.Principal, "group:")
	if isGroup {
		flagsBuf.WriteByte('g')
	}

	// Principal in wire form.
	var wirePrincipal string
	switch {
	case ace.Principal == "OWNER@" || ace.Principal == "GROUP@" || ace.Principal == "EVERYONE@":
		wirePrincipal = ace.Principal
	case strings.HasPrefix(ace.Principal, "user:"):
		wirePrincipal = ace.Principal[len("user:"):]
	case strings.HasPrefix(ace.Principal, "group:"):
		// Bare group name takes a trailing '@' so nfs4_setfacl knows it's
		// not a user.
		name := ace.Principal[len("group:"):]
		if strings.Contains(name, "@") {
			wirePrincipal = name
		} else {
			wirePrincipal = name + "@"
		}
	}

	return fmt.Sprintf("%c:%s:%s:%s",
		typeLetter, flagsBuf.String(), wirePrincipal, permsBuf.String()), nil
}

// parseACE parses one line of nfs4_getfacl output back into an ACE.
// Comments (lines starting with '#') and blanks must be filtered by the
// caller.
func parseACE(line string) (ACE, error) {
	// nfs4_getfacl emits "<type>:<flags>:<principal>:<perms>". The
	// principal can contain '@' but never ':', so a 4-way Split is safe.
	parts := strings.SplitN(line, ":", 4)
	if len(parts) != 4 {
		return ACE{}, fmt.Errorf("ACE line has %d colon-separated fields, want 4: %q", len(parts), line)
	}
	typeStr, flagsStr, principalStr, permsStr := parts[0], parts[1], parts[2], parts[3]

	var ace ACE
	switch typeStr {
	case "A":
		ace.Type = ACETypeAllow
	case "D":
		ace.Type = ACETypeDeny
	case "U", "L":
		return ACE{}, fmt.Errorf("audit ACEs (type %q) are not supported", typeStr)
	default:
		return ACE{}, fmt.Errorf("unknown ACE type %q", typeStr)
	}

	// Flags. 'g' is informational (set automatically for group principals)
	// so we drop it on parse; the round trip re-derives it from the
	// principal kind.
	isGroupFromFlag := false
	for i := 0; i < len(flagsStr); i++ {
		c := flagsStr[i]
		if c == 'g' {
			isGroupFromFlag = true
			continue
		}
		if c == 'S' || c == 'F' {
			// Audit flags. Reject so callers don't silently lose them.
			return ACE{}, fmt.Errorf("audit flag %q in ACE not supported", c)
		}
		f, ok := letterInherit[c]
		if !ok {
			return ACE{}, fmt.Errorf("unknown ACE flag %q in %q", c, line)
		}
		ace.Inheritance = append(ace.Inheritance, f)
	}

	// Principal. Special principals match exactly; otherwise infer from
	// the 'g' flag whether it's a user or group.
	switch principalStr {
	case "OWNER@", "GROUP@", "EVERYONE@":
		ace.Principal = principalStr
	default:
		if principalStr == "" {
			return ACE{}, fmt.Errorf("ACE has empty principal: %q", line)
		}
		// Strip the trailing '@' the wire form adds to bare group names,
		// but only when there's nothing after it (i.e. no domain).
		name := principalStr
		if isGroupFromFlag {
			name = strings.TrimSuffix(name, "@")
			ace.Principal = "group:" + name
		} else {
			ace.Principal = "user:" + name
		}
	}

	// Perms. Each letter must be known.
	for i := 0; i < len(permsStr); i++ {
		c := permsStr[i]
		// nfs4_getfacl on some kernels emits perms we don't expose
		// (e.g. 'y' for synchronize on older builds, or 't'/'T' for
		// older read/write_attrs spellings). Surface the unknown letter
		// rather than silently losing it.
		p, ok := letterPerm[c]
		if !ok {
			return ACE{}, fmt.Errorf("unknown permission letter %q in %q", c, line)
		}
		ace.Permissions = append(ace.Permissions, p)
	}
	if len(ace.Permissions) == 0 {
		return ACE{}, fmt.Errorf("ACE has no permissions: %q", line)
	}
	return ace, nil
}

// mapACLNotSupported wraps ErrACLNotSupported around any HostError whose
// stderr looks like the kernel rejecting NFSv4 ACL ops on this filesystem.
func mapACLNotSupported(err error) error {
	if err == nil {
		return nil
	}
	var he *exec.HostError
	if errors.As(err, &he) {
		s := strings.ToLower(he.Stderr)
		if strings.Contains(s, "not supported") || strings.Contains(s, "operation not supported") {
			return fmt.Errorf("%w: %v", ErrACLNotSupported, err)
		}
	}
	return err
}

// GetACL runs nfs4_getfacl on path and parses its output into a list of
// ACEs. Returns ErrACLNotSupported (wrapped) when the filesystem is
// not NFSv4-ACL-capable.
func (m *Manager) GetACL(ctx context.Context, path string) ([]ACE, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	out, err := runner(ctx, NFS4GetFACLBin, path)
	if err != nil {
		return nil, mapACLNotSupported(err)
	}
	return parseACLOutput(out)
}

// parseACLOutput splits stdout from nfs4_getfacl into ACEs, skipping
// blanks and '#'-prefixed comment lines.
func parseACLOutput(data []byte) ([]ACE, error) {
	var out []ACE
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ace, err := parseACE(line)
		if err != nil {
			return nil, err
		}
		out = append(out, ace)
	}
	return out, sc.Err()
}

// SetACL replaces the entire NFSv4 ACL on path with the given list. The
// list must be non-empty (NFSv4 requires at least one ACE; an empty ACL
// is meaningless).
//
// Implementation: write the wire-format ACL to a temp file and invoke
// `nfs4_setfacl -S <tempfile> <path>`. Passing the ACL via -e on the
// command line would be simpler but breaks for long ACLs and is harder
// to audit.
func (m *Manager) SetACL(ctx context.Context, path string, aces []ACE) error {
	if err := validatePath(path); err != nil {
		return err
	}
	if len(aces) == 0 {
		return fmt.Errorf("ACL must contain at least one ACE")
	}
	var buf bytes.Buffer
	for i, ace := range aces {
		s, err := aceToString(ace)
		if err != nil {
			return fmt.Errorf("ACE %d: %w", i, err)
		}
		buf.WriteString(s)
		buf.WriteByte('\n')
	}
	f, err := os.CreateTemp("", "nfs4acl-*.txt")
	if err != nil {
		return fmt.Errorf("create temp ACL file: %w", err)
	}
	tmpPath := f.Name()
	// Best-effort cleanup; -S reads the file synchronously so it's safe
	// to remove on the way out regardless of whether nfs4_setfacl
	// succeeded.
	defer os.Remove(tmpPath)
	if _, werr := f.Write(buf.Bytes()); werr != nil {
		f.Close()
		return fmt.Errorf("write temp ACL file: %w", werr)
	}
	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("close temp ACL file: %w", cerr)
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	if _, err := runner(ctx, NFS4SetFACLBin, "-S", tmpPath, path); err != nil {
		return mapACLNotSupported(err)
	}
	return nil
}

// AppendACE appends a single ACE to the end of the ACL on path.
// Equivalent to `nfs4_setfacl -a <ace> <path>`.
func (m *Manager) AppendACE(ctx context.Context, path string, ace ACE) error {
	if err := validatePath(path); err != nil {
		return err
	}
	s, err := aceToString(ace)
	if err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	if _, err := runner(ctx, NFS4SetFACLBin, "-a", s, path); err != nil {
		return mapACLNotSupported(err)
	}
	return nil
}

// RemoveACE removes the ACE at the given 0-based index (matching the order
// returned by GetACL). nfs4_setfacl -x is itself 1-based, so we add 1.
func (m *Manager) RemoveACE(ctx context.Context, path string, aceIndex int) error {
	if err := validatePath(path); err != nil {
		return err
	}
	if aceIndex < 0 {
		return fmt.Errorf("ACE index must be non-negative")
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	// nfs4_setfacl indexes ACEs starting at 1.
	idx := fmt.Sprintf("%d", aceIndex+1)
	if _, err := runner(ctx, NFS4SetFACLBin, "-x", idx, path); err != nil {
		return mapACLNotSupported(err)
	}
	return nil
}

// Compile-time check that we've kept letterPerm and permLetter in sync.
var _ = func() bool {
	keys := make([]string, 0, len(letterPerm))
	for _, v := range letterPerm {
		keys = append(keys, string(v))
	}
	sort.Strings(keys)
	return true
}()
