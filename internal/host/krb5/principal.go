// Package krb5 — KDC principal management via kadmin.local.
//
// All operations shell out to `kadmin.local -q "<cmd>"`. Local-DB access
// is unauthenticated; this is safe because kadmin.local is run as root
// inside nova-api's privileged process tree on the KDC host.
package krb5

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PrincipalInfo is a minimal principal description returned by GetPrincipal
// and (singleton list) by CreatePrincipal. The full kadmin "get_principal"
// output is far richer, but we only project the fields the API surfaces.
type PrincipalInfo struct {
	Name       string `json:"name"`
	KVNO       int    `json:"kvno,omitempty"`
	Expiration string `json:"expiration,omitempty"`
	Attributes string `json:"attributes,omitempty"`
}

// CreatePrincipalSpec is the input to CreatePrincipal.
type CreatePrincipalSpec struct {
	// Name is the unqualified principal name (e.g. "nfs/host.example.com")
	// or fully-qualified ("nfs/host.example.com@NOVANAS.LOCAL"). When
	// unqualified, the KDC's default realm is appended at lookup time.
	Name string
	// Randkey, when true, generates a random key (the typical case for
	// service principals — they only need a keytab, never a password).
	Randkey bool
	// Password, when non-empty, sets an initial password. Mutually
	// exclusive with Randkey. If both are unset, Randkey is implied.
	Password string
}

// principalNameRE permits the conservative shape of an MIT principal:
// alphanumerics, dots, dashes, underscores, slashes (component separator),
// and an optional @REALM. We deliberately reject whitespace, shell
// metacharacters, and quotes so the value is safe to embed in a
// `kadmin.local -q "..."` argument.
var principalNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/\-]*(@[A-Za-z0-9][A-Za-z0-9._\-]*)?$`)

func validatePrincipalName(name string) error {
	if name == "" {
		return errors.New("krb5: principal name is required")
	}
	if len(name) > 256 {
		return errors.New("krb5: principal name too long")
	}
	if !principalNameRE.MatchString(name) {
		return fmt.Errorf("krb5: principal name %q is not valid", name)
	}
	return nil
}

// CreatePrincipal adds a principal via `kadmin.local -q add_principal`.
// Returns the parsed info. If a principal with the same name already
// exists, the underlying kadmin error is surfaced verbatim.
func (m *KDCManager) CreatePrincipal(ctx context.Context, spec CreatePrincipalSpec) (*PrincipalInfo, error) {
	if err := validatePrincipalName(spec.Name); err != nil {
		return nil, err
	}
	if spec.Randkey && spec.Password != "" {
		return nil, errors.New("krb5: randkey and password are mutually exclusive")
	}
	// Default to randkey for service principals when neither flag is set.
	useRandkey := spec.Randkey || spec.Password == ""

	var query string
	switch {
	case useRandkey:
		query = fmt.Sprintf("add_principal -randkey %s", spec.Name)
	default:
		// -pw avoids kadmin's interactive password prompt. The password
		// flows through argv (visible to /proc/<pid>/cmdline for root only
		// on this host) — acceptable for the local-DB path where the
		// caller already has root-equivalent API access.
		query = fmt.Sprintf("add_principal -pw %s %s", shellQuote(spec.Password), spec.Name)
	}
	if _, err := m.run(ctx, m.kadminLocalBin(), "-q", query); err != nil {
		return nil, fmt.Errorf("kadmin add_principal: %w", err)
	}
	return m.GetPrincipal(ctx, spec.Name)
}

// DeletePrincipal removes a principal. Returns nil if the principal does
// not exist after the call (idempotent on the "not found" tail of kadmin's
// stderr); other errors propagate.
func (m *KDCManager) DeletePrincipal(ctx context.Context, name string) error {
	if err := validatePrincipalName(name); err != nil {
		return err
	}
	query := fmt.Sprintf("delete_principal -force %s", name)
	if _, err := m.run(ctx, m.kadminLocalBin(), "-q", query); err != nil {
		// kadmin returns non-zero with "Principal does not exist" — treat
		// as idempotent success.
		if isPrincipalNotFound(err) {
			return nil
		}
		return fmt.Errorf("kadmin delete_principal: %w", err)
	}
	return nil
}

// ListPrincipals returns the set of principal names known to the KDC.
func (m *KDCManager) ListPrincipals(ctx context.Context) ([]string, error) {
	out, err := m.run(ctx, m.kadminLocalBin(), "-q", "list_principals")
	if err != nil {
		return nil, fmt.Errorf("kadmin list_principals: %w", err)
	}
	return parseListPrincipals(out), nil
}

// GetPrincipal fetches one principal's projection. Returns fs.ErrNotExist-
// flavored error wrapped so callers can errors.Is it for 404 mapping.
func (m *KDCManager) GetPrincipal(ctx context.Context, name string) (*PrincipalInfo, error) {
	if err := validatePrincipalName(name); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("get_principal %s", name)
	out, err := m.run(ctx, m.kadminLocalBin(), "-q", query)
	if err != nil {
		if isPrincipalNotFound(err) {
			return nil, fmt.Errorf("krb5: principal %q: %w", name, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("kadmin get_principal: %w", err)
	}
	info := parseGetPrincipal(out)
	if info == nil {
		return nil, fmt.Errorf("krb5: principal %q: %w", name, fs.ErrNotExist)
	}
	if info.Name == "" {
		info.Name = name
	}
	return info, nil
}

// GenerateKeytab produces a keytab containing the current keys for name and
// returns its raw bytes. ktadd writes to a file; we route to a fresh path
// in dir, read it, then clean up. dir defaults to os.TempDir() when empty.
//
// IMPORTANT: ktadd by default *increments* the principal's KVNO. Existing
// keytabs distributed to clients become invalid. Callers that need
// non-rotating reads should snapshot the principal externally.
func (m *KDCManager) GenerateKeytab(ctx context.Context, name, dir string) ([]byte, error) {
	if err := validatePrincipalName(name); err != nil {
		return nil, err
	}
	if dir == "" {
		dir = os.TempDir()
	}
	// We tag the file with a sanitized version of the principal name. The
	// validator already restricted the charset, but '/' is meaningful in
	// principal names (component separator) and not on disk, so swap it.
	safeName := strings.ReplaceAll(name, "/", "_")
	tmp, err := os.CreateTemp(dir, "novanas-keytab-"+safeName+"-*.keytab")
	if err != nil {
		return nil, fmt.Errorf("create temp keytab: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	// kadmin won't overwrite an existing keytab; remove the empty placeholder.
	_ = os.Remove(tmpPath)
	defer func() { _ = os.Remove(tmpPath) }()

	query := fmt.Sprintf("ktadd -k %s %s", tmpPath, name)
	if _, err := m.run(ctx, m.kadminLocalBin(), "-q", query); err != nil {
		return nil, fmt.Errorf("kadmin ktadd: %w", err)
	}
	data, err := os.ReadFile(filepath.Clean(tmpPath)) //nolint:gosec // tmpPath is created above
	if err != nil {
		return nil, fmt.Errorf("read keytab: %w", err)
	}
	if len(data) == 0 || data[0] != keytabMagic {
		return nil, errors.New("krb5: ktadd produced empty or invalid keytab")
	}
	return data, nil
}

// shellQuote wraps a single-line value in single quotes, escaping embedded
// single quotes. We use it for the password argument to `kadmin.local -q`,
// which is parsed as a shell-like token by kadmin's command processor.
//
// We intentionally accept only printable characters in passwords — this is
// enforced by the API handler, not here, so we keep this helper narrow.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parseListPrincipals extracts non-empty trimmed lines from kadmin output,
// skipping the "Authenticating as principal..." banner kadmin prints to
// stdout in some builds.
func parseListPrincipals(data []byte) []string {
	out := []string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Authenticating as principal") {
			continue
		}
		// kadmin sometimes prefixes "kadmin.local: " noise to log lines.
		if strings.HasPrefix(line, "kadmin.local:") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// parseGetPrincipal handles the "Principal: <name>\nExpiration date: ...\n..."
// shape kadmin emits for `get_principal`. We only project a few fields; an
// unrecognized line is ignored. Returns nil if no Principal: line is found.
func parseGetPrincipal(data []byte) *PrincipalInfo {
	info := &PrincipalInfo{}
	found := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		k, v, ok := splitColon(line)
		if !ok {
			continue
		}
		switch k {
		case "Principal":
			info.Name = v
			found = true
		case "Expiration date":
			info.Expiration = v
		case "Attributes":
			info.Attributes = v
		case "Key version":
			// "Key: vno N, ..." formatted in some builds. We accept simple
			// integer-only values; everything else is dropped.
			n := atoiSafe(strings.Fields(v))
			if n > 0 {
				info.KVNO = n
			}
		}
	}
	if !found {
		return nil
	}
	return info
}

func splitColon(s string) (string, string, bool) {
	idx := strings.IndexByte(s, ':')
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

// atoiSafe returns the first parseable positive integer in fields, or 0.
func atoiSafe(fields []string) int {
	for _, f := range fields {
		n := 0
		valid := false
		for _, c := range f {
			if c < '0' || c > '9' {
				valid = false
				break
			}
			n = n*10 + int(c-'0')
			valid = true
		}
		if valid && n > 0 {
			return n
		}
	}
	return 0
}

// isPrincipalNotFound returns true if err looks like a kadmin "principal
// does not exist" diagnostic. We match on stderr because the exit code is
// non-specific.
func isPrincipalNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "principal does not exist") ||
		strings.Contains(s, "not found in kerberos database")
}
