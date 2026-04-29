package samba

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// globalsFileName is the basename of the managed globals drop-in. The
// "00-" prefix sorts ahead of any per-share file ("nova-nas-*"), so
// Samba's `include = ...d/*.conf` glob loads the [global] section
// before any [share] section.
const globalsFileName = "00-nova-globals.conf"

// GlobalsOpts controls cross-protocol Samba globals. The defaults are
// tuned for ZFS + NFSv4 ACLs + cross-protocol (SMB+NFS) sharing of the
// same dataset, mirroring what TrueNAS Scale and Synology DSM ship.
type GlobalsOpts struct {
	// Workgroup is the SMB workgroup or NetBIOS domain. Default "WORKGROUP".
	Workgroup string `json:"workgroup,omitempty"`

	// ServerString is the textual description shown to SMB browsers.
	// Default "NovaNAS".
	ServerString string `json:"serverString,omitempty"`

	// ACLProfile selects the ACL behavior. "nfsv4" enables vfs_zfsacl
	// and the cross-protocol-safe globals (recommended). "posix" leaves
	// Samba in legacy POSIX-ACL mode (NFS-only datasets).
	ACLProfile string `json:"aclProfile,omitempty"` // "nfsv4" (default) | "posix"

	// SecurityMode: "user" (default) | "ads" (active directory).
	SecurityMode string `json:"securityMode,omitempty"`

	// Realm is the AD/Kerberos realm when SecurityMode="ads".
	Realm string `json:"realm,omitempty"`

	// EnableNetBIOS controls the smbd NetBIOS layer. Default false
	// (NetBIOS is legacy and noisy on the network).
	EnableNetBIOS bool `json:"enableNetbios,omitempty"`

	// CustomLines are operator escape-hatches: extra "key = value"
	// lines appended verbatim to the [global] section. Each entry must
	// be a single line. Use sparingly.
	CustomLines []string `json:"customLines,omitempty"`
}

// globalsFilePath returns the absolute path of the managed globals
// drop-in inside ConfigDir.
func (m *Manager) globalsFilePath() string {
	return filepath.Join(m.dir(), globalsFileName)
}

// applyGlobalsDefaults fills in the documented defaults for a zero-value
// GlobalsOpts. Returns a copy so the caller's struct is not mutated.
func applyGlobalsDefaults(o GlobalsOpts) GlobalsOpts {
	if o.Workgroup == "" {
		o.Workgroup = "WORKGROUP"
	}
	if o.ServerString == "" {
		o.ServerString = "NovaNAS"
	}
	if o.ACLProfile == "" {
		o.ACLProfile = "nfsv4"
	}
	if o.SecurityMode == "" {
		o.SecurityMode = "user"
	}
	return o
}

// ---------- validation ----------

// validateWorkgroup: NetBIOS-ish — 1-15 chars, alnum + a few separators,
// no whitespace, no shell metas. Samba is permissive in practice but we
// keep this conservative since the value lands in smb.conf.
func validateWorkgroup(w string) error {
	if w == "" {
		return fmt.Errorf("workgroup required")
	}
	if len(w) > 15 {
		return fmt.Errorf("workgroup too long (>15): %q", w)
	}
	for _, r := range w {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !ok {
			return fmt.Errorf("workgroup contains invalid character %q in %q", r, w)
		}
	}
	return nil
}

// validateServerString: no newlines / control chars, ≤ 256 chars. Same
// shape as comment validation on shares.
func validateServerString(s string) error {
	if len(s) > 256 {
		return fmt.Errorf("server string too long (>256)")
	}
	for _, r := range s {
		if r == '\n' || r == '\r' || r == 0 {
			return fmt.Errorf("server string contains forbidden character %q", r)
		}
	}
	return nil
}

// validateRealm: dotted alnum + dash. Empty is OK (only required when
// SecurityMode=ads, which we check at the call site).
func validateRealm(r string) error {
	if r == "" {
		return nil
	}
	if len(r) > 255 {
		return fmt.Errorf("realm too long (>255): %q", r)
	}
	for _, c := range r {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.'
		if !ok {
			return fmt.Errorf("realm contains invalid character %q in %q", c, r)
		}
	}
	return nil
}

// validateCustomLine: must contain '=', no newline, no NUL, no shell
// metacharacters that could break out of the smb.conf line.
func validateCustomLine(line string) error {
	if !strings.Contains(line, "=") {
		return fmt.Errorf("custom line must contain '=': %q", line)
	}
	for _, r := range line {
		if r == '\n' || r == '\r' || r == 0 {
			return fmt.Errorf("custom line contains forbidden control character: %q", line)
		}
		switch r {
		case ';', '|', '&', '$', '`', '\\', '<', '>', '\x00':
			return fmt.Errorf("custom line contains forbidden shell metacharacter %q: %q", r, line)
		}
	}
	return nil
}

func validateGlobals(o GlobalsOpts) error {
	if err := validateWorkgroup(o.Workgroup); err != nil {
		return err
	}
	if err := validateServerString(o.ServerString); err != nil {
		return err
	}
	switch o.ACLProfile {
	case "nfsv4", "posix":
	default:
		return fmt.Errorf("aclProfile must be \"nfsv4\" or \"posix\": %q", o.ACLProfile)
	}
	switch o.SecurityMode {
	case "user", "ads":
	default:
		return fmt.Errorf("securityMode must be \"user\" or \"ads\": %q", o.SecurityMode)
	}
	if err := validateRealm(o.Realm); err != nil {
		return err
	}
	if o.SecurityMode == "ads" && o.Realm == "" {
		return fmt.Errorf("realm required when securityMode=ads")
	}
	for i, line := range o.CustomLines {
		if err := validateCustomLine(line); err != nil {
			return fmt.Errorf("customLines[%d]: %w", i, err)
		}
	}
	return nil
}

// ---------- rendering ----------

// renderGlobalsConf writes the [global] section. Indented 3 spaces with
// spaces around `=`, matching the per-share file convention.
func renderGlobalsConf(o GlobalsOpts) []byte {
	var b bytes.Buffer
	b.WriteString("[global]\n")
	fmt.Fprintf(&b, "   workgroup = %s\n", o.Workgroup)
	fmt.Fprintf(&b, "   server string = %s\n", o.ServerString)
	b.WriteString("   server min protocol = SMB2\n")
	b.WriteString("   server max protocol = SMB3\n")

	if o.ACLProfile == "nfsv4" {
		// client min protocol is only documented in the nfsv4 profile
		// rendering; keep it scoped there to match the spec.
		b.WriteString("   client min protocol = SMB2\n")
	}

	fmt.Fprintf(&b, "   security = %s\n", o.SecurityMode)
	if o.SecurityMode == "ads" {
		fmt.Fprintf(&b, "   realm = %s\n", o.Realm)
	}

	// NetBIOS — disabled by default (legacy). Only emit the disable
	// line when EnableNetBIOS is false.
	if !o.EnableNetBIOS {
		b.WriteString("   disable netbios = yes\n")
	}

	if o.ACLProfile == "nfsv4" {
		b.WriteString("   vfs objects = zfsacl\n")
		b.WriteString("   nfs4: chown = yes\n")
		b.WriteString("   nfs4: acedup = merge\n")
		b.WriteString("   nfs4: mode = special\n")
		b.WriteString("   nt acl support = yes\n")
		b.WriteString("   inherit acls = yes\n")
		b.WriteString("   map acl inherit = yes\n")
		b.WriteString("   store dos attributes = yes\n")
		b.WriteString("   ea support = yes\n")
		b.WriteString("   oplocks = no\n")
		b.WriteString("   level2 oplocks = no\n")
		b.WriteString("   kernel oplocks = no\n")
		b.WriteString("   idmap config * : backend = tdb\n")
		b.WriteString("   idmap config * : range = 100000-200000\n")
	} else { // posix
		b.WriteString("   vfs objects = acl_xattr\n")
		b.WriteString("   acl_xattr:ignore system acls = yes\n")
		b.WriteString("   nt acl support = yes\n")
		b.WriteString("   store dos attributes = yes\n")
	}

	for _, line := range o.CustomLines {
		fmt.Fprintf(&b, "   %s\n", strings.TrimSpace(line))
	}
	return b.Bytes()
}

// ---------- public API ----------

// SetGlobals validates opts (with defaults applied), writes the managed
// globals drop-in (<ConfigDir>/00-nova-globals.conf, mode 0644), runs
// `testparm -s` on the merged config, and reloads smbd. On testparm
// failure the previous file content is restored (or the file removed if
// it did not previously exist) — same rollback shape as CreateShare.
func (m *Manager) SetGlobals(ctx context.Context, opts GlobalsOpts) error {
	opts = applyGlobalsDefaults(opts)
	if err := validateGlobals(opts); err != nil {
		return err
	}
	path := m.globalsFilePath()

	// Capture previous state for rollback. ErrNotExist is fine — that
	// means there's nothing to restore.
	prev, prevErr := m.fw().ReadFile(path)
	hadPrev := prevErr == nil
	if prevErr != nil && !errors.Is(prevErr, os.ErrNotExist) {
		return fmt.Errorf("read existing globals: %w", prevErr)
	}

	if err := m.fw().Write(path, renderGlobalsConf(opts), 0o644); err != nil {
		return fmt.Errorf("write globals: %w", err)
	}
	if err := m.testparm(ctx); err != nil {
		// Restore previous content / remove the new file.
		if hadPrev {
			_ = m.fw().Write(path, prev, 0o644)
		} else {
			_ = m.fw().Remove(path)
		}
		return fmt.Errorf("testparm validation failed: %w", err)
	}
	if err := m.reload(ctx); err != nil {
		return fmt.Errorf("reload smbd: %w", err)
	}
	return nil
}

// GetGlobals reads the managed globals drop-in and parses it back into
// a GlobalsOpts. If the file does not exist a zero-value GlobalsOpts is
// returned with no error (not-yet-configured is not an error). The
// parser is best-effort: only the keys we render are extracted; unknown
// keys are ignored.
func (m *Manager) GetGlobals(ctx context.Context) (*GlobalsOpts, error) {
	data, err := m.fw().ReadFile(m.globalsFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &GlobalsOpts{}, nil
		}
		return nil, fmt.Errorf("read globals: %w", err)
	}
	out, err := parseGlobalsConf(data)
	if err != nil {
		return nil, fmt.Errorf("parse globals: %w", err)
	}
	return out, nil
}

// ---------- parser ----------

// parseGlobalsConf reads the keys we render in renderGlobalsConf back
// into a GlobalsOpts. Unknown keys (and the assorted nfs4/idmap/oplock
// noise we always emit) are silently dropped — those are
// implementation details of the profile, not user input.
func parseGlobalsConf(data []byte) (*GlobalsOpts, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	out := &GlobalsOpts{}
	headerSeen := false
	// Heuristic: presence of "vfs objects = zfsacl" => nfsv4 profile,
	// "vfs objects = acl_xattr" => posix. Default to nfsv4 if neither
	// is seen (the profile is the dominant default).
	sawZFS := false
	sawACLXattr := false
	// EnableNetBIOS: we emit "disable netbios = yes" when disabled, and
	// nothing when enabled. So default to "enabled" and flip false on
	// seeing the disable line.
	disableNetBIOSSeen := false

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			headerSeen = true
			continue
		}
		if !headerSeen {
			// Tolerate stray content before the header rather than erroring.
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		switch key {
		case "workgroup":
			out.Workgroup = val
		case "server string":
			out.ServerString = val
		case "security":
			out.SecurityMode = val
		case "realm":
			out.Realm = val
		case "disable netbios":
			disableNetBIOSSeen = parseBool(val)
		case "vfs objects":
			if strings.Contains(val, "zfsacl") {
				sawZFS = true
			}
			if strings.Contains(val, "acl_xattr") {
				sawACLXattr = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !headerSeen {
		return nil, fmt.Errorf("no [global] header found")
	}
	switch {
	case sawZFS:
		out.ACLProfile = "nfsv4"
	case sawACLXattr:
		out.ACLProfile = "posix"
	default:
		out.ACLProfile = "nfsv4"
	}
	out.EnableNetBIOS = !disableNetBIOSSeen
	return out, nil
}
