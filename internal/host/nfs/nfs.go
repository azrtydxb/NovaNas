// Package nfs manages NFS exports by writing files under /etc/exports.d/
// and reloading the kernel exports table via `exportfs -ra`.
//
// Each managed export is materialized as a single file
// /etc/exports.d/<FilePrefix><Name>.exports containing one line in the
// standard exports(5) format:
//
//	<Path> <Spec1>(<Options1>) <Spec2>(<Options2>) ...
//
// We deliberately keep one path per file. Multi-path exports inside a
// single file are not produced by us; the parser is forgiving (it accepts
// the first non-comment, non-blank line) so a file we re-read will round
// trip even if an operator added comments. We do not attempt to support
// multi-line per-export forms — each file owns exactly one export.
package nfs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// ErrNotFound is returned when GetExport cannot find a file for the
// requested name in ExportsDir.
var ErrNotFound = errors.New("export not found")

// Export is a publishable NFS share.
type Export struct {
	Name    string       `json:"name"`    // filename-safe; identifies the export file
	Path    string       `json:"path"`    // host path (e.g. /tank/share1)
	Clients []ClientRule `json:"clients"`
}

// ClientRule pairs an address pattern with NFS export options.
type ClientRule struct {
	// Spec is a CIDR ("10.0.0.0/24"), an IP ("10.0.0.5"), "*" (any),
	// or a hostname/wildcard ("*.example.com").
	Spec string `json:"spec"`
	// Options is a comma-separated NFS option list, e.g.
	// "rw,sync,root_squash,sec=sys" or "rw,sec=krb5p,fsid=0".
	Options string `json:"options"`
}

// ActiveExport is one row from `exportfs -v` output.
type ActiveExport struct {
	Path    string
	Client  string
	Options string
}

// FileWriter abstracts file ops so tests can stub them.
type FileWriter interface {
	Write(path string, data []byte, perm os.FileMode) error
	Remove(path string) error
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]os.DirEntry, error)
}

// osFileWriter is the default FileWriter backed by package os.
type osFileWriter struct{}

func (osFileWriter) Write(path string, data []byte, perm os.FileMode) error {
	// /etc/exports.d may not exist on a fresh distro install — nfs-utils
	// only ships the directory under nfs-server's runtime, not the
	// package. Best-effort MkdirAll here so first-write succeeds without
	// requiring the operator to create the dir manually.
	if dir := filepath.Dir(path); dir != "" && dir != "." && dir != "/" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(path, data, perm)
}
func (osFileWriter) Remove(path string) error { return os.Remove(path) }
func (osFileWriter) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (osFileWriter) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// Manager manages NFS exports under /etc/exports.d.
type Manager struct {
	ExportsBin string // default "/usr/sbin/exportfs"
	ExportsDir string // default "/etc/exports.d"
	FilePrefix string // default "nova-nas-" — name namespace for our managed files
	// RequireKerberos, when true, enforces sec=krb5p as the default for
	// every export. ClientRules whose Options already contain a sec=
	// token are left untouched (the explicit caller wins). Defaults to
	// false to preserve pre-Kerberos behavior; NovaNAS deployment flips
	// it on once the KDC and host keytab are present.
	RequireKerberos bool
	Runner          exec.Runner
	FileWriter      FileWriter
}

func (m *Manager) bin() string {
	if m.ExportsBin == "" {
		return "/usr/sbin/exportfs"
	}
	return m.ExportsBin
}

func (m *Manager) dir() string {
	if m.ExportsDir == "" {
		return "/etc/exports.d"
	}
	return m.ExportsDir
}

func (m *Manager) prefix() string {
	if m.FilePrefix == "" {
		return "nova-nas-"
	}
	return m.FilePrefix
}

func (m *Manager) fw() FileWriter {
	if m.FileWriter == nil {
		return osFileWriter{}
	}
	return m.FileWriter
}

func (m *Manager) run(ctx context.Context, args ...string) ([]byte, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	return runner(ctx, m.bin(), args...)
}

// filePath returns the absolute path of the exports file owned by name.
func (m *Manager) filePath(name string) string {
	return filepath.Join(m.dir(), m.prefix()+name+".exports")
}

// ---------- validation ----------

// validateExportName: 1-64 chars, alphanumeric + dash + underscore only,
// no leading dash.
func validateExportName(name string) error {
	if name == "" {
		return fmt.Errorf("export name required")
	}
	if len(name) > 64 {
		return fmt.Errorf("export name too long (>64): %q", name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("export name cannot start with '-': %q", name)
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("export name contains invalid character %q in %q", r, name)
		}
	}
	return nil
}

// validateExportPath: absolute path, no .., no NUL, no whitespace, no
// shell metacharacters that could survive into a child process.
func validateExportPath(p string) error {
	if p == "" {
		return fmt.Errorf("export path required")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("export path must be absolute: %q", p)
	}
	if strings.Contains(p, "\x00") {
		return fmt.Errorf("export path contains NUL")
	}
	// Disallow ".." anywhere as a path component.
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("export path must not contain '..': %q", p)
		}
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("export path contains control character: %q", p)
		}
		switch r {
		case ' ', '\t', ';', '|', '&', '$', '`', '\\', '"', '\'', '<', '>', '*', '?', '(', ')', '{', '}', '[', ']':
			return fmt.Errorf("export path contains forbidden character %q: %q", r, p)
		}
	}
	return nil
}

// validateClientSpec accepts CIDR, IP, "*", or hostname/wildcard pattern.
func validateClientSpec(s string) error {
	if s == "" {
		return fmt.Errorf("client spec required")
	}
	if s == "*" {
		return nil
	}
	if strings.Contains(s, "/") {
		if _, _, err := net.ParseCIDR(s); err != nil {
			return fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		return nil
	}
	if ip := net.ParseIP(s); ip != nil {
		return nil
	}
	// Hostname / wildcard pattern: alnum, '.', '-', '*', '?'. Reject
	// leading dash to prevent argv-flag-ish injection in any future
	// caller.
	if strings.HasPrefix(s, "-") {
		return fmt.Errorf("client spec cannot start with '-': %q", s)
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '*' || r == '?'
		if !ok {
			return fmt.Errorf("client spec contains invalid character %q in %q", r, s)
		}
	}
	return nil
}

// validateExportOptions: comma-separated list, restricted alphabet, ≤ 256
// chars. Rejects any character that could escape the file line into shell
// (the file is consumed by the kernel via exportfs, but we still avoid
// surprises like backticks or quotes that would confuse human review or
// future tooling).
func validateExportOptions(opts string) error {
	if opts == "" {
		return fmt.Errorf("export options required")
	}
	if len(opts) > 256 {
		return fmt.Errorf("export options too long (>256)")
	}
	for _, r := range opts {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '=' || r == ',' || r == '/' || r == '-' || r == '_' || r == ':'
		if !ok {
			return fmt.Errorf("export options contain invalid character %q in %q", r, opts)
		}
	}
	return nil
}

func validateExport(e Export) error {
	if err := validateExportName(e.Name); err != nil {
		return err
	}
	if err := validateExportPath(e.Path); err != nil {
		return err
	}
	if len(e.Clients) == 0 {
		return fmt.Errorf("at least one client rule required")
	}
	for i, c := range e.Clients {
		if err := validateClientSpec(c.Spec); err != nil {
			return fmt.Errorf("client[%d]: %w", i, err)
		}
		if err := validateExportOptions(c.Options); err != nil {
			return fmt.Errorf("client[%d]: %w", i, err)
		}
	}
	return nil
}

// applyKerberosDefault prepends "sec=krb5p" to every client rule's
// option list when RequireKerberos is true and the caller has not
// already specified a sec= token. Caller-provided sec=... values win
// (explicit callers can opt down to sec=krb5i / sec=sys for one-off
// exports while the global policy still requires krb5p as a default).
//
// We mutate a copy of e — never the caller's slice — so behavior with
// repeated calls or shared Export structs stays predictable.
func (m *Manager) applyKerberosDefault(e Export) Export {
	if !m.RequireKerberos {
		return e
	}
	out := e
	out.Clients = make([]ClientRule, len(e.Clients))
	for i, c := range e.Clients {
		if hasSecOption(c.Options) {
			out.Clients[i] = c
			continue
		}
		opts := c.Options
		if opts == "" {
			opts = "sec=krb5p"
		} else {
			opts = "sec=krb5p," + opts
		}
		out.Clients[i] = ClientRule{Spec: c.Spec, Options: opts}
	}
	return out
}

// hasSecOption reports whether the comma-separated option list contains
// a "sec=..." token. Tokens are matched case-insensitively on the key
// to mirror nfs-utils which accepts SEC= as well as sec=.
func hasSecOption(opts string) bool {
	for _, tok := range strings.Split(opts, ",") {
		k := strings.TrimSpace(tok)
		if k == "" {
			continue
		}
		if eq := strings.IndexByte(k, '='); eq >= 0 {
			k = k[:eq]
		}
		if strings.EqualFold(k, "sec") {
			return true
		}
	}
	return false
}

// ---------- file rendering ----------

// renderExportFile builds the single-line file body for an Export. A
// trailing newline is appended so editors behave.
func renderExportFile(e Export) []byte {
	var b bytes.Buffer
	b.WriteString(e.Path)
	for _, c := range e.Clients {
		b.WriteByte(' ')
		b.WriteString(c.Spec)
		b.WriteByte('(')
		b.WriteString(c.Options)
		b.WriteByte(')')
	}
	b.WriteByte('\n')
	return b.Bytes()
}

// ---------- public API ----------

// CreateExport validates e, writes the exports file (failing if it
// already exists), and reloads the kernel table.
func (m *Manager) CreateExport(ctx context.Context, e Export) error {
	e = m.applyKerberosDefault(e)
	if err := validateExport(e); err != nil {
		return err
	}
	path := m.filePath(e.Name)
	// Reject duplicate: the file must not already exist. We use ReadFile
	// through the FileWriter so tests can stub it; real os.ReadFile maps
	// missing files to an error wrapping fs.ErrNotExist, which we treat
	// as the success-to-create case.
	if _, err := m.fw().ReadFile(path); err == nil {
		return fmt.Errorf("export %q already exists", e.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat existing export %q: %w", e.Name, err)
	}
	if err := m.fw().Write(path, renderExportFile(e), 0o644); err != nil {
		return fmt.Errorf("write export %q: %w", e.Name, err)
	}
	if _, err := m.run(ctx, "-ra"); err != nil {
		return fmt.Errorf("exportfs -ra: %w", err)
	}
	return nil
}

// UpdateExport validates e, requires the file already exists, overwrites
// its contents, and reloads.
func (m *Manager) UpdateExport(ctx context.Context, e Export) error {
	e = m.applyKerberosDefault(e)
	if err := validateExport(e); err != nil {
		return err
	}
	path := m.filePath(e.Name)
	if _, err := m.fw().ReadFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("read existing export %q: %w", e.Name, err)
	}
	if err := m.fw().Write(path, renderExportFile(e), 0o644); err != nil {
		return fmt.Errorf("write export %q: %w", e.Name, err)
	}
	if _, err := m.run(ctx, "-ra"); err != nil {
		return fmt.Errorf("exportfs -ra: %w", err)
	}
	return nil
}

// DeleteExport removes the named exports file and reloads. Missing file
// is reported as ErrNotFound.
func (m *Manager) DeleteExport(ctx context.Context, name string) error {
	if err := validateExportName(name); err != nil {
		return err
	}
	path := m.filePath(name)
	if err := m.fw().Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("remove export %q: %w", name, err)
	}
	if _, err := m.run(ctx, "-ra"); err != nil {
		return fmt.Errorf("exportfs -ra: %w", err)
	}
	return nil
}

// ListExports reads ExportsDir, filters to files we own (FilePrefix +
// .exports suffix), and parses each. Files we cannot parse are skipped
// silently — operator-edited noise should not block the API.
func (m *Manager) ListExports(ctx context.Context) ([]Export, error) {
	entries, err := m.fw().ReadDir(m.dir())
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", m.dir(), err)
	}
	prefix := m.prefix()
	out := make([]Export, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		fname := ent.Name()
		if !strings.HasPrefix(fname, prefix) || !strings.HasSuffix(fname, ".exports") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(fname, prefix), ".exports")
		if validateExportName(name) != nil {
			continue
		}
		data, err := m.fw().ReadFile(filepath.Join(m.dir(), fname))
		if err != nil {
			continue
		}
		ex, err := parseExportFile(data)
		if err != nil {
			continue
		}
		ex.Name = name
		out = append(out, *ex)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetExport reads the single named exports file. Missing is ErrNotFound.
func (m *Manager) GetExport(ctx context.Context, name string) (*Export, error) {
	if err := validateExportName(name); err != nil {
		return nil, err
	}
	data, err := m.fw().ReadFile(m.filePath(name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read export %q: %w", name, err)
	}
	ex, err := parseExportFile(data)
	if err != nil {
		return nil, fmt.Errorf("parse export %q: %w", name, err)
	}
	ex.Name = name
	return ex, nil
}

// Reload runs `exportfs -ra`. Useful when callers edited ExportsDir
// directly and want to commit the change.
func (m *Manager) Reload(ctx context.Context) error {
	if _, err := m.run(ctx, "-ra"); err != nil {
		return fmt.Errorf("exportfs -ra: %w", err)
	}
	return nil
}

// ListActive runs `exportfs -v` and parses the output.
func (m *Manager) ListActive(ctx context.Context) ([]ActiveExport, error) {
	out, err := m.run(ctx, "-v")
	if err != nil {
		return nil, fmt.Errorf("exportfs -v: %w", err)
	}
	return parseExportfsV(out)
}

// ---------- parsers ----------

// parseExportFile parses one of our managed files. We accept the first
// non-blank, non-comment line of the form:
//
//	<Path> <Spec>(<Opts>) <Spec>(<Opts>) ...
//
// Comments (#-prefixed) and blank lines are ignored. Subsequent
// non-blank lines are not interpreted (we only emit one).
func parseExportFile(data []byte) (*Export, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return parseExportLine(line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("no export line found")
}

// parseExportLine parses a single line: "<Path> <Spec>(<Opts>) ...".
func parseExportLine(line string) (*Export, error) {
	// Path is the first whitespace-delimited token.
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return nil, fmt.Errorf("export line missing client list: %q", line)
	}
	path := line[:idx]
	rest := strings.TrimSpace(line[idx+1:])
	if rest == "" {
		return nil, fmt.Errorf("export line has no clients: %q", line)
	}
	clients, err := parseClientList(rest)
	if err != nil {
		return nil, err
	}
	return &Export{Path: path, Clients: clients}, nil
}

// parseClientList parses tokens like "10.0.0.0/24(rw,sync) *(ro)" into
// ClientRule slices. Whitespace separates tokens; each token must end
// with ")".
func parseClientList(s string) ([]ClientRule, error) {
	var out []ClientRule
	tokens := strings.Fields(s)
	for _, tok := range tokens {
		op := strings.IndexByte(tok, '(')
		if op < 0 || !strings.HasSuffix(tok, ")") {
			return nil, fmt.Errorf("malformed client token %q", tok)
		}
		spec := tok[:op]
		opts := tok[op+1 : len(tok)-1]
		if spec == "" {
			return nil, fmt.Errorf("client token missing spec: %q", tok)
		}
		out = append(out, ClientRule{Spec: spec, Options: opts})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no client rules parsed from %q", s)
	}
	return out, nil
}

// parseExportfsV parses `exportfs -v` output. The canonical format is:
//
//	/tank/share    10.0.0.0/24(rw,sync,root_squash,no_subtree_check)
//
// exportfs may wrap long lines (path on one line, indented "<tab>client(opts)"
// on the next). We handle both: when a line starts with whitespace and we
// have a previously seen path, we attribute the line to that path.
func parseExportfsV(data []byte) ([]ActiveExport, error) {
	var out []ActiveExport
	var lastPath string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		raw := sc.Text()
		if strings.TrimSpace(raw) == "" {
			continue
		}
		indented := len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t')
		line := strings.TrimSpace(raw)
		if indented && lastPath != "" {
			// Continuation: only a "<client>(<opts>)" token.
			ae, err := parseActiveToken(lastPath, line)
			if err != nil {
				continue
			}
			out = append(out, ae)
			continue
		}
		// Path + (optional) client on the same line.
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		lastPath = fields[0]
		if len(fields) == 1 {
			continue
		}
		// Remaining fields each form a "<client>(<opts>)" token.
		for _, tok := range fields[1:] {
			ae, err := parseActiveToken(lastPath, tok)
			if err != nil {
				continue
			}
			out = append(out, ae)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseActiveToken(path, tok string) (ActiveExport, error) {
	op := strings.IndexByte(tok, '(')
	if op < 0 || !strings.HasSuffix(tok, ")") {
		return ActiveExport{}, fmt.Errorf("malformed active token %q", tok)
	}
	return ActiveExport{
		Path:    path,
		Client:  tok[:op],
		Options: tok[op+1 : len(tok)-1],
	}, nil
}
