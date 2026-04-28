// Package iscsi wraps /usr/bin/targetcli to manage iSCSI target lifecycle:
// backstores, targets (IQNs), portals, LUNs, and ACLs (with optional CHAP).
//
// Invocation form: targetcli is invoked with separate argv elements per
// segment, e.g. `targetcli /iscsi create wwn=iqn.2020-01.io.example:tank`.
// We pass each segment (path, command, kwargs) as a separate argv element
// rather than a single quoted positional. This matches what `exec.Run`
// expects (no shell) and was verified against Debian's targetcli on the
// dev box: both single-positional and split-argv forms parse identically,
// but the split form avoids any quoting concerns. The one quirk is `ls`:
// passing a positional integer for depth is interpreted as a path
// component, so we use the kwarg form `depth=N` instead.
//
// targetcli must be invoked as root. Tests stub the runner via a captured
// argv recorder.
package iscsi

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// Manager performs iSCSI target operations via targetcli.
type Manager struct {
	TargetcliBin string
	Runner       exec.Runner
}

// Target identifies an iSCSI target by IQN. v1 always uses tpg1; the
// multi-TPG case is intentionally not exposed.
type Target struct {
	IQN string `json:"iqn"`
}

// Portal is a (ip, port, transport) triple bound to a target's tpg1.
type Portal struct {
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Transport string `json:"transport"` // "tcp" | "iser"
}

// LUN maps a backstore (block-backed by a zvol) into a target.
type LUN struct {
	ID        int    `json:"id"`
	Zvol      string `json:"zvol"`      // /dev/zvol/<pool>/<name>
	Backstore string `json:"backstore"` // sanitized backstore name
}

// ACL is an initiator entry on a target's tpg1 with optional CHAP.
// CHAPSecret is never returned in reads; GetTarget blanks it.
type ACL struct {
	InitiatorIQN string `json:"initiatorIqn"`
	CHAPUser     string `json:"chapUser,omitempty"`
	CHAPSecret   string `json:"chapSecret,omitempty"`
}

// TargetDetail is the aggregated view of a target (portals, LUNs, ACLs).
// CHAPSecret on ACLs is always blanked.
type TargetDetail struct {
	Target  Target   `json:"target"`
	Portals []Portal `json:"portals"`
	LUNs    []LUN    `json:"luns"`
	ACLs    []ACL    `json:"acls"`
}

func (m *Manager) bin() string {
	if m.TargetcliBin == "" {
		return "/usr/bin/targetcli"
	}
	return m.TargetcliBin
}

func (m *Manager) run(ctx context.Context, args ...string) ([]byte, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	return runner(ctx, m.bin(), args...)
}

// ---------- validation helpers ----------

// validateNoDash rejects strings starting with '-' to prevent argv-flag
// injection through user-controlled values that get passed positionally.
func validateNoDash(field, v string) error {
	if strings.HasPrefix(v, "-") {
		return fmt.Errorf("%s cannot start with '-' (argv injection guard)", field)
	}
	return nil
}

// validateNoWhitespace rejects any whitespace.
func validateNoWhitespace(field, v string) error {
	for _, r := range v {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return fmt.Errorf("%s cannot contain whitespace", field)
		}
	}
	return nil
}

// validateBackstoreName: alphanumeric plus '_' '-' '.'; no '/', no spaces,
// no leading dash.
func validateBackstoreName(name string) error {
	if name == "" {
		return fmt.Errorf("backstore name required")
	}
	if err := validateNoDash("backstore name", name); err != nil {
		return err
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !ok {
			return fmt.Errorf("backstore name contains invalid character %q", r)
		}
	}
	return nil
}

// validateDevPath: must start with /dev/zvol/, no whitespace, no leading
// dash, no shell metacharacters that could survive into a child process.
func validateDevPath(p string) error {
	if !strings.HasPrefix(p, "/dev/zvol/") {
		return fmt.Errorf("dev path must start with /dev/zvol/")
	}
	if err := validateNoDash("dev path", p); err != nil {
		return err
	}
	if err := validateNoWhitespace("dev path", p); err != nil {
		return err
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("dev path contains control character")
		}
		switch r {
		case ';', '|', '&', '$', '`', '\\', '"', '\'', '<', '>', '*', '?', '(', ')', '{', '}', '[', ']':
			return fmt.Errorf("dev path contains shell metacharacter %q", r)
		}
	}
	return nil
}

// validateIQN: starts with "iqn.", restricted alphabet (alnum, '.', '-',
// ':'), no leading dash, no whitespace.
func validateIQN(iqn string) error {
	if !strings.HasPrefix(iqn, "iqn.") {
		return fmt.Errorf("IQN must start with 'iqn.'")
	}
	if err := validateNoDash("IQN", iqn); err != nil {
		return err
	}
	for _, r := range iqn {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == ':'
		if !ok {
			return fmt.Errorf("IQN contains invalid character %q", r)
		}
	}
	return nil
}

// validatePortal validates ip / port / transport.
func validatePortal(p Portal) error {
	if net.ParseIP(p.IP) == nil {
		return fmt.Errorf("invalid IP %q", p.IP)
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("port out of range: %d", p.Port)
	}
	switch p.Transport {
	case "", "tcp", "iser":
	default:
		return fmt.Errorf("invalid transport %q (want tcp|iser)", p.Transport)
	}
	return nil
}

// validateCHAPField rejects control chars and shell metacharacters in
// CHAP user/secret values. iSCSI also requires CHAP secrets to be 12-16
// characters (RFC 3720).
func validateCHAPField(field, v string) error {
	if v == "" {
		return fmt.Errorf("%s required", field)
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains control character", field)
		}
		switch r {
		case ' ', '\t', ';', '|', '&', '$', '`', '\\', '"', '\'', '<', '>', '*', '?', '(', ')', '{', '}', '[', ']', '=':
			return fmt.Errorf("%s contains forbidden character %q", field, r)
		}
	}
	return nil
}

// ---------- argv builders ----------

func buildCreateBackstoreArgs(name, devPath string) ([]string, error) {
	if err := validateBackstoreName(name); err != nil {
		return nil, err
	}
	if err := validateDevPath(devPath); err != nil {
		return nil, err
	}
	return []string{"/backstores/block", "create", "name=" + name, "dev=" + devPath}, nil
}

func buildDeleteBackstoreArgs(name string) ([]string, error) {
	if err := validateBackstoreName(name); err != nil {
		return nil, err
	}
	return []string{"/backstores/block", "delete", "name=" + name}, nil
}

func buildCreateTargetArgs(iqn string) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	return []string{"/iscsi", "create", "wwn=" + iqn}, nil
}

func buildDeleteTargetArgs(iqn string) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	return []string{"/iscsi", "delete", "wwn=" + iqn}, nil
}

func buildListTargetsArgs() []string {
	return []string{"/iscsi", "ls", "depth=1"}
}

func buildGetTargetArgs(iqn string) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	return []string{"/iscsi/" + iqn, "ls"}, nil
}

func buildCreatePortalArgs(iqn string, p Portal) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if err := validatePortal(p); err != nil {
		return nil, err
	}
	return []string{
		"/iscsi/" + iqn + "/tpg1/portals", "create",
		"ip_address=" + p.IP, "ip_port=" + strconv.Itoa(p.Port),
	}, nil
}

func buildEnableIserArgs(iqn string, p Portal) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if err := validatePortal(p); err != nil {
		return nil, err
	}
	return []string{
		"/iscsi/" + iqn + "/tpg1/portals/" + p.IP + ":" + strconv.Itoa(p.Port),
		"enable_iser", "true",
	}, nil
}

func buildDeletePortalArgs(iqn string, p Portal) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if err := validatePortal(p); err != nil {
		return nil, err
	}
	return []string{
		"/iscsi/" + iqn + "/tpg1/portals", "delete",
		"ip_address=" + p.IP, "ip_port=" + strconv.Itoa(p.Port),
	}, nil
}

func buildCreateLUNArgs(iqn string, lun LUN) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if lun.ID < 0 {
		return nil, fmt.Errorf("LUN id must be >= 0")
	}
	if err := validateBackstoreName(lun.Backstore); err != nil {
		return nil, err
	}
	return []string{
		"/iscsi/" + iqn + "/tpg1/luns", "create",
		"storage_object=/backstores/block/" + lun.Backstore,
		"lun=" + strconv.Itoa(lun.ID),
	}, nil
}

func buildDeleteLUNArgs(iqn string, id int) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if id < 0 {
		return nil, fmt.Errorf("LUN id must be >= 0")
	}
	return []string{"/iscsi/" + iqn + "/tpg1/luns", "delete", strconv.Itoa(id)}, nil
}

func buildCreateACLArgs(iqn string, acl ACL) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if err := validateIQN(acl.InitiatorIQN); err != nil {
		return nil, fmt.Errorf("initiator IQN: %w", err)
	}
	return []string{
		"/iscsi/" + iqn + "/tpg1/acls", "create",
		"wwn=" + acl.InitiatorIQN,
	}, nil
}

func buildSetCHAPUserArgs(iqn string, acl ACL) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if err := validateIQN(acl.InitiatorIQN); err != nil {
		return nil, fmt.Errorf("initiator IQN: %w", err)
	}
	if err := validateCHAPField("CHAP user", acl.CHAPUser); err != nil {
		return nil, err
	}
	return []string{
		"/iscsi/" + iqn + "/tpg1/acls/" + acl.InitiatorIQN,
		"set", "auth", "userid=" + acl.CHAPUser,
	}, nil
}

func buildSetCHAPPasswordArgs(iqn string, acl ACL) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if err := validateIQN(acl.InitiatorIQN); err != nil {
		return nil, fmt.Errorf("initiator IQN: %w", err)
	}
	if err := validateCHAPField("CHAP secret", acl.CHAPSecret); err != nil {
		return nil, err
	}
	if n := len(acl.CHAPSecret); n < 12 || n > 16 {
		return nil, fmt.Errorf("CHAP secret must be 12-16 characters (got %d)", n)
	}
	return []string{
		"/iscsi/" + iqn + "/tpg1/acls/" + acl.InitiatorIQN,
		"set", "auth", "password=" + acl.CHAPSecret,
	}, nil
}

func buildDeleteACLArgs(iqn, initiatorIQN string) ([]string, error) {
	if err := validateIQN(iqn); err != nil {
		return nil, err
	}
	if err := validateIQN(initiatorIQN); err != nil {
		return nil, fmt.Errorf("initiator IQN: %w", err)
	}
	return []string{
		"/iscsi/" + iqn + "/tpg1/acls", "delete", "wwn=" + initiatorIQN,
	}, nil
}

func buildSaveConfigArgs() []string {
	return []string{"saveconfig"}
}

// ---------- public API ----------

// CreateBackstore creates a block backstore named `name` pointing at
// `devPath` (which must live under /dev/zvol/).
func (m *Manager) CreateBackstore(ctx context.Context, name, devPath string) error {
	args, err := buildCreateBackstoreArgs(name, devPath)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// DeleteBackstore removes the named block backstore.
func (m *Manager) DeleteBackstore(ctx context.Context, name string) error {
	args, err := buildDeleteBackstoreArgs(name)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// CreateTarget creates a new iSCSI target with the given IQN.
//
// targetcli's default behavior is to also create a 0.0.0.0:3260 portal
// at target-create time. NovaNAS's API gives the operator explicit
// control over portals (NIC choice, multipath), so we delete that
// auto-created portal immediately. The deletion is best-effort — newer
// targetcli versions sometimes skip the auto-add when no default portal
// would bind.
func (m *Manager) CreateTarget(ctx context.Context, iqn string) error {
	args, err := buildCreateTargetArgs(iqn)
	if err != nil {
		return err
	}
	if _, err := m.run(ctx, args...); err != nil {
		return err
	}
	// Best-effort: remove the auto-created 0.0.0.0:3260 portal so the
	// caller's first explicit CreatePortal won't conflict.
	_, _ = m.run(ctx, "/iscsi/"+iqn+"/tpg1/portals", "delete",
		"ip_address=0.0.0.0", "ip_port=3260")
	return nil
}

// DeleteTarget removes an iSCSI target by IQN.
func (m *Manager) DeleteTarget(ctx context.Context, iqn string) error {
	args, err := buildDeleteTargetArgs(iqn)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// ListTargets returns all targets visible to targetcli (depth=1).
func (m *Manager) ListTargets(ctx context.Context) ([]Target, error) {
	out, err := m.run(ctx, buildListTargetsArgs()...)
	if err != nil {
		return nil, err
	}
	return parseTargetList(out)
}

// GetTarget returns the full detail (portals, LUNs, ACLs) for a target.
// CHAP secrets are never returned, even if visible in the underlying
// targetcli output.
func (m *Manager) GetTarget(ctx context.Context, iqn string) (*TargetDetail, error) {
	args, err := buildGetTargetArgs(iqn)
	if err != nil {
		return nil, err
	}
	out, err := m.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	detail, err := parseTargetDetail(out)
	if err != nil {
		return nil, err
	}
	detail.Target.IQN = iqn
	// Defensive: blank any CHAP secret the parser might have surfaced.
	for i := range detail.ACLs {
		detail.ACLs[i].CHAPSecret = ""
	}
	return detail, nil
}

// CreatePortal binds a portal (ip:port, optional iSER) to the target's tpg1.
func (m *Manager) CreatePortal(ctx context.Context, iqn string, p Portal) error {
	args, err := buildCreatePortalArgs(iqn, p)
	if err != nil {
		return err
	}
	if _, err := m.run(ctx, args...); err != nil {
		return err
	}
	if p.Transport == "iser" {
		args, err := buildEnableIserArgs(iqn, p)
		if err != nil {
			return err
		}
		if _, err := m.run(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

// DeletePortal removes a portal binding.
func (m *Manager) DeletePortal(ctx context.Context, iqn string, p Portal) error {
	args, err := buildDeletePortalArgs(iqn, p)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// CreateLUN attaches a backstore to a target as the given LUN id.
// The backstore must already exist (call CreateBackstore first).
func (m *Manager) CreateLUN(ctx context.Context, iqn string, lun LUN) error {
	args, err := buildCreateLUNArgs(iqn, lun)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// DeleteLUN detaches a LUN by id from the target's tpg1.
func (m *Manager) DeleteLUN(ctx context.Context, iqn string, id int) error {
	args, err := buildDeleteLUNArgs(iqn, id)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// CreateACL adds an initiator ACL to the target's tpg1 and, if CHAP
// credentials are provided, configures auth.
func (m *Manager) CreateACL(ctx context.Context, iqn string, acl ACL) error {
	args, err := buildCreateACLArgs(iqn, acl)
	if err != nil {
		return err
	}
	if acl.CHAPUser != "" || acl.CHAPSecret != "" {
		// Validate both halves up front so we don't half-create.
		if _, err := buildSetCHAPUserArgs(iqn, acl); err != nil {
			return err
		}
		if _, err := buildSetCHAPPasswordArgs(iqn, acl); err != nil {
			return err
		}
	}
	if _, err := m.run(ctx, args...); err != nil {
		return err
	}
	if acl.CHAPUser != "" || acl.CHAPSecret != "" {
		userArgs, _ := buildSetCHAPUserArgs(iqn, acl)
		if _, err := m.run(ctx, userArgs...); err != nil {
			return err
		}
		pwArgs, _ := buildSetCHAPPasswordArgs(iqn, acl)
		if _, err := m.run(ctx, pwArgs...); err != nil {
			return err
		}
	}
	return nil
}

// DeleteACL removes an initiator ACL.
func (m *Manager) DeleteACL(ctx context.Context, iqn, initiatorIQN string) error {
	args, err := buildDeleteACLArgs(iqn, initiatorIQN)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, args...)
	return err
}

// SaveConfig persists the running configuration to
// /etc/rtslib-fb-target/saveconfig.json so it survives reboot.
func (m *Manager) SaveConfig(ctx context.Context) error {
	_, err := m.run(ctx, buildSaveConfigArgs()...)
	return err
}
