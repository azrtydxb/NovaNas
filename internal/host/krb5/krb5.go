// Package krb5 manages client-side Kerberos configuration on the host:
// /etc/krb5.conf (realms, KDCs, default realm), /etc/idmapd.conf (NFSv4
// ID-map domain), and /etc/krb5.keytab (host's service principal keys).
//
// NovaNAS does not run its own KDC. Operators bring their own — Active
// Directory, FreeIPA, MIT Kerberos. This Manager only writes the
// client-side config files needed for NFSv4 with sec=krb5* and manages
// the host keytab uploaded by the operator.
//
// File operations are abstracted behind FileSystem so tests can run
// without touching the real /etc paths. The keytab inspection path
// shells out to klist, which is stubbed via exec.Runner in tests.
package krb5

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// Config is the subset of /etc/krb5.conf the Manager owns. Sections
// outside [libdefaults], [realms], and [domain_realm] are not preserved
// when SetConfig rewrites the file — the file is intended to be managed
// by NovaNAS once SetConfig is called.
type Config struct {
	DefaultRealm   string            `json:"defaultRealm"`
	Realms         map[string]Realm  `json:"realms"`
	DomainRealm    map[string]string `json:"domainRealm,omitempty"`
	DNSLookupKDC   bool              `json:"dnsLookupKdc"`
	DNSLookupRealm bool              `json:"dnsLookupRealm"`
}

// Realm describes a single Kerberos realm: its KDCs, optional admin
// server (kadmin), and optional default_domain mapping.
type Realm struct {
	KDC           []string `json:"kdc"`
	AdminServer   string   `json:"adminServer,omitempty"`
	DefaultDomain string   `json:"defaultDomain,omitempty"`
}

// IdmapdConfig is /etc/idmapd.conf, used by NFSv4 to map UIDs to names
// in a Kerberos domain. The Domain field is typically the krb5 default
// realm in lowercase.
type IdmapdConfig struct {
	Domain    string `json:"domain"`
	Verbosity int    `json:"verbosity,omitempty"`
}

// KeytabEntry is one principal stored in /etc/krb5.keytab.
type KeytabEntry struct {
	KVNO       int    `json:"kvno"`
	Principal  string `json:"principal"`
	Encryption string `json:"encryption"`
}

// FileSystem abstracts file I/O so tests can run in-memory without
// touching /etc.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	Stat(path string) (os.FileInfo, error)
	Remove(path string) error
}

// Manager renders and parses Kerberos client-side configuration files
// and inspects/uploads the host keytab.
type Manager struct {
	Krb5ConfPath   string
	KeytabPath     string
	IdmapdConfPath string
	KlistBin       string
	Runner         exec.Runner
	FS             FileSystem
}

func (m *Manager) krb5ConfPath() string {
	if m.Krb5ConfPath == "" {
		return "/etc/krb5.conf"
	}
	return m.Krb5ConfPath
}

func (m *Manager) keytabPath() string {
	if m.KeytabPath == "" {
		return "/etc/krb5.keytab"
	}
	return m.KeytabPath
}

func (m *Manager) idmapdConfPath() string {
	if m.IdmapdConfPath == "" {
		return "/etc/idmapd.conf"
	}
	return m.IdmapdConfPath
}

func (m *Manager) klistBin() string {
	if m.KlistBin == "" {
		return "/usr/bin/klist"
	}
	return m.KlistBin
}

func (m *Manager) fs() FileSystem {
	if m.FS == nil {
		return osFS{}
	}
	return m.FS
}

func (m *Manager) run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	return runner(ctx, bin, args...)
}

// GetConfig reads and parses /etc/krb5.conf. A missing file is not an
// error — it returns a zero-value Config so callers can render a
// "not yet configured" view without branching on os.IsNotExist.
func (m *Manager) GetConfig(ctx context.Context) (*Config, error) {
	data, err := m.fs().ReadFile(m.krb5ConfPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{Realms: map[string]Realm{}}, nil
		}
		return nil, fmt.Errorf("read %s: %w", m.krb5ConfPath(), err)
	}
	return parseKrb5Conf(data)
}

// SetConfig validates cfg and writes a freshly rendered /etc/krb5.conf
// (mode 0644). The file is rewritten atomically.
func (m *Manager) SetConfig(ctx context.Context, cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	data := renderKrb5Conf(cfg)
	return atomicWrite(m.fs(), m.krb5ConfPath(), data, 0o644)
}

// GetIdmapdConfig reads and parses /etc/idmapd.conf. A missing file
// returns a zero-value IdmapdConfig and nil error.
func (m *Manager) GetIdmapdConfig(ctx context.Context) (*IdmapdConfig, error) {
	data, err := m.fs().ReadFile(m.idmapdConfPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &IdmapdConfig{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", m.idmapdConfPath(), err)
	}
	return parseIdmapdConf(data)
}

// SetIdmapdConfig validates and writes a minimal idmapd.conf (mode 0644).
func (m *Manager) SetIdmapdConfig(ctx context.Context, cfg IdmapdConfig) error {
	if err := validateIdmapd(cfg); err != nil {
		return err
	}
	return atomicWrite(m.fs(), m.idmapdConfPath(), renderIdmapdConf(cfg), 0o644)
}

// ListKeytab invokes klist -k -t -e to enumerate principals in the host
// keytab. A missing or empty keytab returns an empty slice and nil error.
func (m *Manager) ListKeytab(ctx context.Context) ([]KeytabEntry, error) {
	if _, err := m.fs().Stat(m.keytabPath()); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []KeytabEntry{}, nil
		}
		return nil, fmt.Errorf("stat %s: %w", m.keytabPath(), err)
	}
	out, err := m.run(ctx, m.klistBin(), "-k", "-t", "-e", m.keytabPath())
	if err != nil {
		return nil, err
	}
	return parseKlistOutput(out)
}

// keytabMagic is the first byte of an MIT-format keytab file (version
// indicator). Both v1 (0x501) and v2 (0x502) keytabs start with 0x05;
// see RFC 4120 / src/lib/krb5/keytab/kt_file.c in MIT krb5.
const keytabMagic = 0x05

// UploadKeytab atomically writes data to the keytab path with mode 0600.
// It rejects empty input and any byte stream that does not start with
// the keytab magic byte (0x05).
func (m *Manager) UploadKeytab(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return errors.New("keytab: empty data")
	}
	if data[0] != keytabMagic {
		return fmt.Errorf("keytab: invalid magic byte 0x%02x (want 0x%02x)", data[0], keytabMagic)
	}
	return atomicWrite(m.fs(), m.keytabPath(), data, 0o600)
}

// DeleteKeytab removes the keytab file. Missing file is not an error.
func (m *Manager) DeleteKeytab(ctx context.Context) error {
	if err := m.fs().Remove(m.keytabPath()); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove %s: %w", m.keytabPath(), err)
	}
	return nil
}

// validateConfig checks the user-facing constraints we enforce on
// krb5.conf input. The realm name is upper-case by convention (RFC 4120
// section 6.1) — we enforce it because mixed case is a frequent
// operator mistake that silently breaks AD integration.
func validateConfig(cfg Config) error {
	if cfg.DefaultRealm == "" {
		return errors.New("krb5: defaultRealm is required")
	}
	if cfg.DefaultRealm != strings.ToUpper(cfg.DefaultRealm) {
		return fmt.Errorf("krb5: defaultRealm %q must be uppercase", cfg.DefaultRealm)
	}
	if !isRealmName(cfg.DefaultRealm) {
		return fmt.Errorf("krb5: defaultRealm %q is not a valid realm name", cfg.DefaultRealm)
	}
	if _, ok := cfg.Realms[cfg.DefaultRealm]; !ok {
		return fmt.Errorf("krb5: defaultRealm %q has no entry in realms", cfg.DefaultRealm)
	}
	for name, r := range cfg.Realms {
		if name != strings.ToUpper(name) {
			return fmt.Errorf("krb5: realm name %q must be uppercase", name)
		}
		if !isRealmName(name) {
			return fmt.Errorf("krb5: realm name %q is invalid", name)
		}
		if len(r.KDC) == 0 {
			return fmt.Errorf("krb5: realm %q must have at least one KDC", name)
		}
		for _, kdc := range r.KDC {
			if err := validateHostPort(kdc); err != nil {
				return fmt.Errorf("krb5: realm %q kdc %q: %w", name, kdc, err)
			}
		}
		if r.AdminServer != "" {
			if err := validateHostPort(r.AdminServer); err != nil {
				return fmt.Errorf("krb5: realm %q adminServer %q: %w", name, r.AdminServer, err)
			}
		}
		if r.DefaultDomain != "" && !isDNSName(r.DefaultDomain) {
			return fmt.Errorf("krb5: realm %q defaultDomain %q is not a valid DNS name", name, r.DefaultDomain)
		}
	}
	for suffix, realm := range cfg.DomainRealm {
		// suffix may be ".example.com" or "example.com"
		s := strings.TrimPrefix(suffix, ".")
		if !isDNSName(s) {
			return fmt.Errorf("krb5: domain_realm key %q is not a valid DNS suffix", suffix)
		}
		if realm != strings.ToUpper(realm) || !isRealmName(realm) {
			return fmt.Errorf("krb5: domain_realm value %q is not a valid uppercase realm", realm)
		}
	}
	return nil
}

func validateIdmapd(cfg IdmapdConfig) error {
	if cfg.Domain == "" {
		return errors.New("idmapd: domain is required")
	}
	if !isDNSName(cfg.Domain) {
		return fmt.Errorf("idmapd: domain %q is not a valid DNS name", cfg.Domain)
	}
	if cfg.Verbosity < 0 || cfg.Verbosity > 9 {
		return fmt.Errorf("idmapd: verbosity %d out of range 0-9", cfg.Verbosity)
	}
	return nil
}

// realmNameRE accepts the conservative shape we render: alphanumerics,
// dots, dashes, underscores. Matches what AD/FreeIPA/MIT all produce.
var realmNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func isRealmName(s string) bool { return realmNameRE.MatchString(s) }

// dnsLabelRE matches a single DNS label.
var dnsLabelRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$`)

func isDNSName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, lbl := range strings.Split(s, ".") {
		if !dnsLabelRE.MatchString(lbl) {
			return false
		}
	}
	return true
}

// validateHostPort accepts "host" or "host:port". The host part must
// be a DNS name or IPv4/IPv6 address; the port must be 1-65535.
func validateHostPort(s string) error {
	if s == "" {
		return errors.New("empty")
	}
	host := s
	port := ""
	// IPv6 with port: [::1]:88
	if strings.HasPrefix(s, "[") {
		h, p, err := net.SplitHostPort(s)
		if err != nil {
			return fmt.Errorf("parse host:port: %w", err)
		}
		host, port = h, p
	} else if strings.Count(s, ":") == 1 {
		h, p, err := net.SplitHostPort(s)
		if err != nil {
			return fmt.Errorf("parse host:port: %w", err)
		}
		host, port = h, p
	}
	if net.ParseIP(host) == nil && !isDNSName(host) {
		return fmt.Errorf("host %q is not a valid IP or DNS name", host)
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("port %q is not in 1-65535", port)
		}
	}
	return nil
}

// atomicWrite writes data to path via a "<path>.tmp" sibling, fsyncing
// before rename. This pattern matches nvmeof.SaveToFile and keeps the
// file from ending up half-written if the process crashes.
//
// It calls FileSystem.WriteFile for the temp file — when FS is the real
// filesystem we additionally fsync via os primitives; when FS is an
// in-memory test stub fsync is a no-op.
func atomicWrite(fsys FileSystem, path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp := path + ".tmp"
	if err := fsys.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write temp %s: %w", tmp, err)
	}
	// If we're on the real filesystem, fsync + atomic rename. The
	// in-memory test stub overrides WriteFile to do its own rename.
	if real, ok := fsys.(osFS); ok {
		if err := real.fsync(tmp); err != nil {
			_ = fsys.Remove(tmp)
			return fmt.Errorf("fsync %s: %w", tmp, err)
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = fsys.Remove(tmp)
			return fmt.Errorf("rename %s: %w", tmp, err)
		}
		// Best-effort dirsync.
		if d, err := os.Open(dir); err == nil {
			_ = d.Sync()
			_ = d.Close()
		}
		return nil
	}
	// Test stub: do the rename through Stat/Read/Write/Remove only.
	data2, err := fsys.ReadFile(tmp)
	if err != nil {
		return fmt.Errorf("read temp %s: %w", tmp, err)
	}
	if err := fsys.WriteFile(path, data2, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	_ = fsys.Remove(tmp)
	return nil
}

// osFS is the production FileSystem implementation backed by package os.
type osFS struct{}

func (osFS) ReadFile(p string) ([]byte, error)               { return os.ReadFile(p) }
func (osFS) WriteFile(p string, d []byte, m os.FileMode) error { return os.WriteFile(p, d, m) }
func (osFS) Stat(p string) (os.FileInfo, error)              { return os.Stat(p) }
func (osFS) Remove(p string) error                           { return os.Remove(p) }

func (osFS) fsync(p string) error {
	f, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// --- parsers / renderers --------------------------------------------------

// parseKrb5Conf is a deliberately narrow INI-ish parser. Real krb5.conf
// files in the wild use nested braces (the realms section opens a brace
// per realm) and { } on their own lines. We only need to handle the
// shape our renderer produces plus the common operator-typed variants:
//   * leading/trailing whitespace
//   * comments starting with '#' or ';'
//   * section headers in [brackets]
//   * key = value lines
//   * "REALM = {" opening a nested block
//   * lines containing only "}" closing the nested block
// Anything else is ignored.
func parseKrb5Conf(data []byte) (*Config, error) {
	cfg := &Config{Realms: map[string]Realm{}, DomainRealm: map[string]string{}}
	lines := strings.Split(string(data), "\n")

	section := ""
	currentRealm := ""
	var realm Realm

	flushRealm := func() {
		if currentRealm != "" {
			cfg.Realms[currentRealm] = realm
		}
		currentRealm = ""
		realm = Realm{}
	}

	for _, raw := range lines {
		line := stripComment(strings.TrimSpace(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flushRealm()
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if line == "}" {
			flushRealm()
			continue
		}

		switch section {
		case "libdefaults":
			k, v, ok := splitKV(line)
			if !ok {
				continue
			}
			switch k {
			case "default_realm":
				cfg.DefaultRealm = v
			case "dns_lookup_kdc":
				cfg.DNSLookupKDC = parseBool(v)
			case "dns_lookup_realm":
				cfg.DNSLookupRealm = parseBool(v)
			}
		case "realms":
			// Either "REALM = {" opens a block or we're inside one.
			if currentRealm == "" {
				k, v, ok := splitKV(line)
				if !ok {
					continue
				}
				if v == "{" {
					currentRealm = k
					realm = Realm{}
				}
				continue
			}
			k, v, ok := splitKV(line)
			if !ok {
				continue
			}
			switch k {
			case "kdc":
				realm.KDC = append(realm.KDC, v)
			case "admin_server":
				realm.AdminServer = v
			case "default_domain":
				realm.DefaultDomain = v
			}
		case "domain_realm":
			k, v, ok := splitKV(line)
			if !ok {
				continue
			}
			cfg.DomainRealm[k] = v
		}
	}
	flushRealm()

	if len(cfg.DomainRealm) == 0 {
		cfg.DomainRealm = nil
	}
	return cfg, nil
}

// renderKrb5Conf produces a clean, deterministic krb5.conf. Realms and
// domain_realm entries are emitted in sorted order so byte-identical
// output is produced for identical input.
func renderKrb5Conf(cfg Config) []byte {
	var b bytes.Buffer
	b.WriteString("[libdefaults]\n")
	if cfg.DefaultRealm != "" {
		fmt.Fprintf(&b, "    default_realm = %s\n", cfg.DefaultRealm)
	}
	fmt.Fprintf(&b, "    dns_lookup_kdc = %s\n", boolStr(cfg.DNSLookupKDC))
	fmt.Fprintf(&b, "    dns_lookup_realm = %s\n", boolStr(cfg.DNSLookupRealm))
	b.WriteString("\n[realms]\n")
	for _, name := range sortedKeys(cfg.Realms) {
		r := cfg.Realms[name]
		fmt.Fprintf(&b, "    %s = {\n", name)
		for _, kdc := range r.KDC {
			fmt.Fprintf(&b, "        kdc = %s\n", kdc)
		}
		if r.AdminServer != "" {
			fmt.Fprintf(&b, "        admin_server = %s\n", r.AdminServer)
		}
		if r.DefaultDomain != "" {
			fmt.Fprintf(&b, "        default_domain = %s\n", r.DefaultDomain)
		}
		b.WriteString("    }\n")
	}
	if len(cfg.DomainRealm) > 0 {
		b.WriteString("\n[domain_realm]\n")
		for _, k := range sortedKeys(cfg.DomainRealm) {
			fmt.Fprintf(&b, "    %s = %s\n", k, cfg.DomainRealm[k])
		}
	}
	return b.Bytes()
}

// parseIdmapdConf handles the [General] section — the only one nfsidmap
// needs to interoperate with NFSv4 + Kerberos.
func parseIdmapdConf(data []byte) (*IdmapdConfig, error) {
	cfg := &IdmapdConfig{}
	section := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := stripComment(strings.TrimSpace(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != "General" {
			continue
		}
		k, v, ok := splitKV(line)
		if !ok {
			continue
		}
		switch strings.ToLower(k) {
		case "domain":
			cfg.Domain = v
		case "verbosity":
			n, err := strconv.Atoi(v)
			if err == nil {
				cfg.Verbosity = n
			}
		}
	}
	return cfg, nil
}

func renderIdmapdConf(cfg IdmapdConfig) []byte {
	var b bytes.Buffer
	b.WriteString("[General]\n")
	fmt.Fprintf(&b, "Verbosity = %d\n", cfg.Verbosity)
	fmt.Fprintf(&b, "Domain = %s\n", cfg.Domain)
	b.WriteString("\n[Mapping]\n")
	// No-op mapping section: empty Nobody-User/Group are common but
	// optional; leaving the section header lets operators add overrides.
	return b.Bytes()
}

// klistRowRE matches a klist -k -t -e row. Format:
//   KVNO Timestamp           Principal
//   ---- ------------------- -------------------------------
//      2 04/29/2026 06:18:00 nfs/host.example.com@EXAMPLE.COM (aes256-cts-hmac-sha1-96)
// We accept timestamps in the locale-default (MM/DD/YYYY HH:MM:SS) form
// which is what klist emits with no LC_ALL override on Debian/RHEL.
var klistRowRE = regexp.MustCompile(`^\s*(\d+)\s+\S+\s+\S+\s+(\S+)\s+\(([^)]+)\)\s*$`)

// parseKlistOutput parses the body of `klist -k -t -e <keytab>`. Empty
// input → empty slice. The first non-empty line is expected to be the
// "Keytab name:" header; we skip until the dashed separator row, then
// parse each subsequent row.
func parseKlistOutput(data []byte) ([]KeytabEntry, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return []KeytabEntry{}, nil
	}
	out := []KeytabEntry{}
	sawSeparator := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !sawSeparator {
			// A row of dashes separates the header from the data. Some
			// klist builds emit "----" plus spaces, so we just look for
			// any line that starts with "----".
			if strings.HasPrefix(trimmed, "----") {
				sawSeparator = true
			}
			continue
		}
		m := klistRowRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		kvno, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		out = append(out, KeytabEntry{
			KVNO:       kvno,
			Principal:  m[2],
			Encryption: m[3],
		})
	}
	return out, nil
}

// --- helpers --------------------------------------------------------------

func stripComment(s string) string {
	for i, c := range s {
		if c == '#' || c == ';' {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}

func splitKV(s string) (string, string, bool) {
	idx := strings.IndexByte(s, '=')
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(s[:idx])
	v := strings.TrimSpace(s[idx+1:])
	if k == "" {
		return "", "", false
	}
	return k, v, true
}

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "true", "yes", "1", "on":
		return true
	}
	return false
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// stdlib sort would force another import; bubble sort is fine for
	// the realm-count cardinality we expect (single digits), but use a
	// proper sort for clarity.
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	// Insertion sort: realm/domain counts are small (≤ 10s), so the
	// O(n²) cost is negligible and avoids pulling in "sort" just for
	// this one call.
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
