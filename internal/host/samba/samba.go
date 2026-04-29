// Package samba manages SMB/CIFS shares by writing one Samba config
// snippet per share into a drop-in directory (default
// /etc/samba/smb.conf.d) and reloading the daemon via
// `smbcontrol smbd reload-config`.
//
// # Operator setup
//
// The base /etc/samba/smb.conf must include the drop-in directory.
// nova-nas does not modify smb.conf itself — the operator (or a
// distribution package) is expected to ensure the [global] section
// contains:
//
//	include = /etc/samba/smb.conf.d/*.conf
//
// (Some distributions ship this by default; many do not.) Each managed
// share is materialized as a single file
// /etc/samba/smb.conf.d/<FilePrefix><Name>.conf containing one
// `[<Name>]` section in standard smb.conf syntax (3-space indented
// keys, spaces around `=`).
//
// # Users
//
// Samba users are managed via `smbpasswd` / `pdbedit`. Each samba user
// must map to a Linux system user that already exists; the AddUser /
// SetUserPassword methods only manage the Samba password database, not
// the underlying /etc/passwd entry.
package samba

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// ErrNotFound is returned when GetShare cannot find a file for the
// requested name in ConfigDir.
var ErrNotFound = errors.New("share not found")

// Share is a publishable SMB/CIFS share.
type Share struct {
	Name          string   `json:"name"`
	Path          string   `json:"path"`
	Comment       string   `json:"comment,omitempty"`
	Browseable    bool     `json:"browseable"`
	Writable      bool     `json:"writable"`
	GuestOK       bool     `json:"guestOk"`
	ReadOnly      bool     `json:"readOnly"`
	ValidUsers    []string `json:"validUsers,omitempty"`
	WriteList     []string `json:"writeList,omitempty"`
	AdminUsers    []string `json:"adminUsers,omitempty"`
	CreateMask    string   `json:"createMask,omitempty"`
	DirectoryMask string   `json:"directoryMask,omitempty"`
	Veto          []string `json:"veto,omitempty"`
}

// ActiveShare is one row from `smbstatus -S` output (optional helper).
type ActiveShare struct {
	Name    string
	Service string
	PID     int
	Machine string
	User    string
}

// User is a samba-managed user (smbpasswd database). Maps to a Linux
// system user that must already exist.
type User struct {
	Username string `json:"username"`
}

// FileWriter abstracts file ops so tests can stub them. Same shape as
// the nfs package's FileWriter.
type FileWriter interface {
	Write(path string, data []byte, perm os.FileMode) error
	Remove(path string) error
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]os.DirEntry, error)
}

// osFileWriter is the default FileWriter backed by package os.
type osFileWriter struct{}

func (osFileWriter) Write(path string, data []byte, perm os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." && dir != "/" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(path, data, perm)
}
func (osFileWriter) Remove(path string) error            { return os.Remove(path) }
func (osFileWriter) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (osFileWriter) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// Manager manages SMB/CIFS shares via drop-in files in ConfigDir and
// the Samba CLI tools.
type Manager struct {
	SmbcontrolBin string // default /usr/bin/smbcontrol
	TestparmBin   string // default /usr/bin/testparm
	SmbpasswdBin  string // default /usr/bin/smbpasswd
	PdbeditBin    string // default /usr/bin/pdbedit
	ConfigDir     string // default /etc/samba/smb.conf.d
	FilePrefix    string // default "nova-nas-"
	Runner        exec.Runner
	StdinRunner   exec.StdinRunner
	FileWriter    FileWriter
}

func (m *Manager) smbcontrolBin() string {
	if m.SmbcontrolBin == "" {
		return "/usr/bin/smbcontrol"
	}
	return m.SmbcontrolBin
}

func (m *Manager) testparmBin() string {
	if m.TestparmBin == "" {
		return "/usr/bin/testparm"
	}
	return m.TestparmBin
}

func (m *Manager) smbpasswdBin() string {
	if m.SmbpasswdBin == "" {
		return "/usr/bin/smbpasswd"
	}
	return m.SmbpasswdBin
}

func (m *Manager) pdbeditBin() string {
	if m.PdbeditBin == "" {
		return "/usr/bin/pdbedit"
	}
	return m.PdbeditBin
}

func (m *Manager) dir() string {
	if m.ConfigDir == "" {
		return "/etc/samba/smb.conf.d"
	}
	return m.ConfigDir
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

func (m *Manager) run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	return runner(ctx, bin, args...)
}

func (m *Manager) runStdin(ctx context.Context, bin string, stdin []byte, args ...string) ([]byte, error) {
	runner := m.StdinRunner
	if runner == nil {
		runner = exec.RunStdin
	}
	return runner(ctx, bin, stdin, args...)
}

// filePath returns the absolute path of the share file owned by name.
func (m *Manager) filePath(name string) string {
	return filepath.Join(m.dir(), m.prefix()+name+".conf")
}

// ---------- validation ----------

// validateShareName: 1-64 chars, alphanumeric + dash + underscore only.
// SMB share names are case-insensitive on the wire and must not contain
// whitespace, '/', or NetBIOS-illegal characters. We're stricter than
// the protocol allows on purpose — these names also become filenames.
func validateShareName(name string) error {
	if name == "" {
		return fmt.Errorf("share name required")
	}
	if len(name) > 64 {
		return fmt.Errorf("share name too long (>64): %q", name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("share name cannot start with '-': %q", name)
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("share name contains invalid character %q in %q", r, name)
		}
	}
	return nil
}

// validateSharePath: absolute, no .., no NUL, no whitespace, no shell
// metacharacters that could survive into smb.conf in a confusing way.
func validateSharePath(p string) error {
	if p == "" {
		return fmt.Errorf("share path required")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("share path must be absolute: %q", p)
	}
	if strings.Contains(p, "\x00") {
		return fmt.Errorf("share path contains NUL")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("share path must not contain '..': %q", p)
		}
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("share path contains control character: %q", p)
		}
		// Allow space inside paths? smb.conf can quote paths but we
		// keep it simple and reject. Operators with weird mountpoints
		// can rename them.
		switch r {
		case ' ', '\t', ';', '|', '&', '$', '`', '\\', '"', '\'', '<', '>', '*', '?', '\n', '\r', '[', ']':
			return fmt.Errorf("share path contains forbidden character %q: %q", r, p)
		}
	}
	return nil
}

// validateUsername: Linux user shape. Matches POSIX-portable + common
// extensions: alnum, '.', '-', '_', leading character must be a letter
// or underscore. 1-32 chars. Rejects whitespace and shell metas.
func validateUsername(u string) error {
	if u == "" {
		return fmt.Errorf("username required")
	}
	if len(u) > 32 {
		return fmt.Errorf("username too long (>32): %q", u)
	}
	first := u[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return fmt.Errorf("username must start with a letter or underscore: %q", u)
	}
	for _, r := range u {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !ok {
			return fmt.Errorf("username contains invalid character %q in %q", r, u)
		}
	}
	return nil
}

// validateMask: octal, 3 or 4 digits, all 0-7.
func validateMask(label, m string) error {
	if m == "" {
		return nil
	}
	if len(m) != 3 && len(m) != 4 {
		return fmt.Errorf("%s must be 3 or 4 octal digits: %q", label, m)
	}
	for _, r := range m {
		if r < '0' || r > '7' {
			return fmt.Errorf("%s contains non-octal digit %q: %q", label, r, m)
		}
	}
	return nil
}

// validateVetoEntry: a single component for "veto files = /a/b/c/" —
// no '/' (we add separators), no NUL, no newline.
func validateVetoEntry(e string) error {
	if e == "" {
		return fmt.Errorf("veto entry empty")
	}
	for _, r := range e {
		if r == '/' || r == '\x00' || r == '\n' || r == '\r' {
			return fmt.Errorf("veto entry contains forbidden character %q: %q", r, e)
		}
	}
	return nil
}

// validateComment: no newlines, no control chars, ≤ 256 chars.
func validateComment(c string) error {
	if len(c) > 256 {
		return fmt.Errorf("comment too long (>256)")
	}
	for _, r := range c {
		if r == '\n' || r == '\r' || r == 0 {
			return fmt.Errorf("comment contains forbidden character %q", r)
		}
	}
	return nil
}

func validateShare(s Share) error {
	if err := validateShareName(s.Name); err != nil {
		return err
	}
	if err := validateSharePath(s.Path); err != nil {
		return err
	}
	if err := validateComment(s.Comment); err != nil {
		return err
	}
	if err := validateMask("create mask", s.CreateMask); err != nil {
		return err
	}
	if err := validateMask("directory mask", s.DirectoryMask); err != nil {
		return err
	}
	for i, u := range s.ValidUsers {
		if err := validateUsername(u); err != nil {
			return fmt.Errorf("validUsers[%d]: %w", i, err)
		}
	}
	for i, u := range s.WriteList {
		if err := validateUsername(u); err != nil {
			return fmt.Errorf("writeList[%d]: %w", i, err)
		}
	}
	for i, u := range s.AdminUsers {
		if err := validateUsername(u); err != nil {
			return fmt.Errorf("adminUsers[%d]: %w", i, err)
		}
	}
	for i, v := range s.Veto {
		if err := validateVetoEntry(v); err != nil {
			return fmt.Errorf("veto[%d]: %w", i, err)
		}
	}
	return nil
}

// ---------- rendering ----------

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// renderShareConf produces the smb.conf section for a Share. Keys are
// indented 3 spaces (Samba convention) with spaces around `=`. A
// trailing newline is appended.
func renderShareConf(s Share) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "[%s]\n", s.Name)
	fmt.Fprintf(&b, "   path = %s\n", s.Path)
	if s.Comment != "" {
		fmt.Fprintf(&b, "   comment = %s\n", s.Comment)
	}
	fmt.Fprintf(&b, "   browseable = %s\n", yesno(s.Browseable))
	fmt.Fprintf(&b, "   writable = %s\n", yesno(s.Writable))
	fmt.Fprintf(&b, "   guest ok = %s\n", yesno(s.GuestOK))
	fmt.Fprintf(&b, "   read only = %s\n", yesno(s.ReadOnly))
	if len(s.ValidUsers) > 0 {
		fmt.Fprintf(&b, "   valid users = %s\n", strings.Join(s.ValidUsers, ", "))
	}
	if len(s.WriteList) > 0 {
		fmt.Fprintf(&b, "   write list = %s\n", strings.Join(s.WriteList, ", "))
	}
	if len(s.AdminUsers) > 0 {
		fmt.Fprintf(&b, "   admin users = %s\n", strings.Join(s.AdminUsers, ", "))
	}
	if s.CreateMask != "" {
		fmt.Fprintf(&b, "   create mask = %s\n", s.CreateMask)
	}
	if s.DirectoryMask != "" {
		fmt.Fprintf(&b, "   directory mask = %s\n", s.DirectoryMask)
	}
	if len(s.Veto) > 0 {
		fmt.Fprintf(&b, "   veto files = /%s/\n", strings.Join(s.Veto, "/"))
	}
	return b.Bytes()
}

// ---------- public API ----------

// CreateShare validates s, writes the conf file (failing if it already
// exists), runs `testparm -s` to validate the merged config, and reloads
// smbd via `smbcontrol smbd reload-config`.
func (m *Manager) CreateShare(ctx context.Context, s Share) error {
	if err := validateShare(s); err != nil {
		return err
	}
	path := m.filePath(s.Name)
	if _, err := m.fw().ReadFile(path); err == nil {
		return fmt.Errorf("share %q already exists", s.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat existing share %q: %w", s.Name, err)
	}
	if err := m.fw().Write(path, renderShareConf(s), 0o644); err != nil {
		return fmt.Errorf("write share %q: %w", s.Name, err)
	}
	if err := m.testparm(ctx); err != nil {
		// Roll back the file so a bad share doesn't leave junk
		// behind. Best-effort: the file write succeeded so Remove
		// should as well.
		_ = m.fw().Remove(path)
		return fmt.Errorf("testparm validation failed: %w", err)
	}
	if err := m.reload(ctx); err != nil {
		return fmt.Errorf("reload smbd: %w", err)
	}
	return nil
}

// UpdateShare validates s, requires the file already exists, overwrites
// it, validates, and reloads. On testparm failure the previous content
// is restored.
func (m *Manager) UpdateShare(ctx context.Context, s Share) error {
	if err := validateShare(s); err != nil {
		return err
	}
	path := m.filePath(s.Name)
	prev, err := m.fw().ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("read existing share %q: %w", s.Name, err)
	}
	if err := m.fw().Write(path, renderShareConf(s), 0o644); err != nil {
		return fmt.Errorf("write share %q: %w", s.Name, err)
	}
	if err := m.testparm(ctx); err != nil {
		// Restore previous content. Best-effort.
		_ = m.fw().Write(path, prev, 0o644)
		return fmt.Errorf("testparm validation failed: %w", err)
	}
	if err := m.reload(ctx); err != nil {
		return fmt.Errorf("reload smbd: %w", err)
	}
	return nil
}

// DeleteShare removes the named share file and reloads.
func (m *Manager) DeleteShare(ctx context.Context, name string) error {
	if err := validateShareName(name); err != nil {
		return err
	}
	path := m.filePath(name)
	if err := m.fw().Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("remove share %q: %w", name, err)
	}
	if err := m.reload(ctx); err != nil {
		return fmt.Errorf("reload smbd: %w", err)
	}
	return nil
}

// ListShares reads ConfigDir, filters to files we own (FilePrefix +
// .conf suffix), and parses each. Files we cannot parse are skipped.
func (m *Manager) ListShares(ctx context.Context) ([]Share, error) {
	entries, err := m.fw().ReadDir(m.dir())
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", m.dir(), err)
	}
	prefix := m.prefix()
	out := make([]Share, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		fname := ent.Name()
		if !strings.HasPrefix(fname, prefix) || !strings.HasSuffix(fname, ".conf") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(fname, prefix), ".conf")
		if validateShareName(name) != nil {
			continue
		}
		data, err := m.fw().ReadFile(filepath.Join(m.dir(), fname))
		if err != nil {
			continue
		}
		sh, err := parseShareConf(data)
		if err != nil {
			continue
		}
		// Trust the filename if the section header disagrees.
		sh.Name = name
		out = append(out, *sh)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetShare reads the single named share file. Missing is ErrNotFound.
func (m *Manager) GetShare(ctx context.Context, name string) (*Share, error) {
	if err := validateShareName(name); err != nil {
		return nil, err
	}
	data, err := m.fw().ReadFile(m.filePath(name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read share %q: %w", name, err)
	}
	sh, err := parseShareConf(data)
	if err != nil {
		return nil, fmt.Errorf("parse share %q: %w", name, err)
	}
	sh.Name = name
	return sh, nil
}

// Reload runs `smbcontrol smbd reload-config`.
func (m *Manager) Reload(ctx context.Context) error {
	return m.reload(ctx)
}

func (m *Manager) reload(ctx context.Context) error {
	if _, err := m.run(ctx, m.smbcontrolBin(), "smbd", "reload-config"); err != nil {
		return fmt.Errorf("smbcontrol smbd reload-config: %w", err)
	}
	return nil
}

// testparm runs `testparm -s` (suppress prompts, dump merged config).
// Stdout is the rendered config; on bad config testparm exits non-zero
// with diagnostics on stderr — the HostError surfaces both.
func (m *Manager) testparm(ctx context.Context) error {
	if _, err := m.run(ctx, m.testparmBin(), "-s"); err != nil {
		return err
	}
	return nil
}

// AddUser provisions a samba user. The Linux user must already exist.
// Stdin = "<password>\n<password>\n" (smbpasswd prompts twice).
func (m *Manager) AddUser(ctx context.Context, username, password string) error {
	if err := validateUsername(username); err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password required")
	}
	stdin := []byte(password + "\n" + password + "\n")
	if _, err := m.runStdin(ctx, m.smbpasswdBin(), stdin, "-a", "-s", username); err != nil {
		return fmt.Errorf("smbpasswd -a -s %s: %w", username, err)
	}
	return nil
}

// DeleteUser removes a samba user from the smbpasswd database.
func (m *Manager) DeleteUser(ctx context.Context, username string) error {
	if err := validateUsername(username); err != nil {
		return err
	}
	if _, err := m.run(ctx, m.smbpasswdBin(), "-x", username); err != nil {
		return fmt.Errorf("smbpasswd -x %s: %w", username, err)
	}
	return nil
}

// ListUsers calls `pdbedit -L` and parses one user per line:
// "username:uid:fullname".
func (m *Manager) ListUsers(ctx context.Context) ([]User, error) {
	out, err := m.run(ctx, m.pdbeditBin(), "-L")
	if err != nil {
		return nil, fmt.Errorf("pdbedit -L: %w", err)
	}
	return parsePdbeditL(out), nil
}

// SetUserPassword changes an existing samba user's password. Stdin =
// "<newPassword>\n<newPassword>\n".
func (m *Manager) SetUserPassword(ctx context.Context, username, newPassword string) error {
	if err := validateUsername(username); err != nil {
		return err
	}
	if newPassword == "" {
		return fmt.Errorf("password required")
	}
	stdin := []byte(newPassword + "\n" + newPassword + "\n")
	if _, err := m.runStdin(ctx, m.smbpasswdBin(), stdin, "-s", username); err != nil {
		return fmt.Errorf("smbpasswd -s %s: %w", username, err)
	}
	return nil
}

// ---------- parsers ----------

// parseShareConf parses one of our managed files. The first
// non-blank, non-comment line must be a "[<name>]" section header.
// Subsequent "key = value" lines populate the Share. Unknown keys are
// ignored. The returned Share's Name is set from the section header
// (callers may override).
func parseShareConf(data []byte) (*Share, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var sh Share
	headerSeen := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if headerSeen {
				// We only emit one section per file; ignore extras.
				break
			}
			sh.Name = strings.TrimSpace(line[1 : len(line)-1])
			headerSeen = true
			continue
		}
		if !headerSeen {
			return nil, fmt.Errorf("expected [section] header before %q", line)
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		switch key {
		case "path":
			sh.Path = val
		case "comment":
			sh.Comment = val
		case "browseable", "browsable":
			sh.Browseable = parseBool(val)
		case "writable", "writeable":
			sh.Writable = parseBool(val)
		case "guest ok":
			sh.GuestOK = parseBool(val)
		case "read only":
			sh.ReadOnly = parseBool(val)
		case "valid users":
			sh.ValidUsers = parseUserList(val)
		case "write list":
			sh.WriteList = parseUserList(val)
		case "admin users":
			sh.AdminUsers = parseUserList(val)
		case "create mask":
			sh.CreateMask = val
		case "directory mask":
			sh.DirectoryMask = val
		case "veto files":
			sh.Veto = parseVetoList(val)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !headerSeen {
		return nil, fmt.Errorf("no [section] header found")
	}
	return &sh, nil
}

// parseBool interprets samba boolean values. "yes"/"true"/"1" => true.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "true", "1", "on":
		return true
	}
	return false
}

// parseUserList splits "user1, user2 user3" into ["user1","user2","user3"].
// Both commas and whitespace are treated as separators.
func parseUserList(s string) []string {
	if s == "" {
		return nil
	}
	repl := strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(repl)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// parseVetoList parses "/a/b/c/" or "/a/b/c" or "a/b/c" into ["a","b","c"].
// Empty components are dropped.
func parseVetoList(s string) []string {
	parts := strings.Split(s, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parsePdbeditL parses `pdbedit -L` output. Each non-blank line is
// "username:uid:fullname". Malformed lines are skipped.
func parsePdbeditL(data []byte) []User {
	var out []User
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Take the username = everything before the first ':'.
		idx := strings.IndexByte(line, ':')
		var u string
		if idx < 0 {
			u = line
		} else {
			u = line[:idx]
		}
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		// Skip names that don't look like Linux usernames (defensive).
		if validateUsername(u) != nil {
			continue
		}
		out = append(out, User{Username: u})
	}
	return out
}
