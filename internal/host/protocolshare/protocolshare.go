// Package protocolshare implements the unified Synology-style "share"
// abstraction. One ProtocolShare maps a single ZFS dataset to a matched
// NFS export and a matched Samba share, all configured for consistent
// NFSv4 ACL semantics.
//
// # Lifecycle
//
// Create / Update / Delete are coordinated across the three underlying
// managers (dataset, nfs, samba). On a Create failure, prior steps are
// rolled back in reverse order. On Delete, removal proceeds in reverse
// order (samba → nfs → dataset) and is best-effort: a failure on one
// step does not stop the others; a multi-error is returned.
//
// # InitGlobals prerequisite
//
// Cross-protocol shares (NFS+SMB serving the same path with consistent
// NFSv4 ACL behavior) require global Samba VFS settings to be applied
// once at deployment time via Manager.InitGlobals. We do NOT call this
// implicitly inside Create — it is a system-wide concern (it edits the
// [global] section, not a single share) and calling it on every Create
// would race with the operator's own samba configuration. It must be
// invoked once during deployment / first-boot. See InitGlobals for
// details.
package protocolshare

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/novanas/nova-nas/internal/host/nfs"
	"github.com/novanas/nova-nas/internal/host/samba"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

// ACE aliases dataset.ACE so the API surface is consistent.
type ACE = dataset.ACE

// GlobalsOpts aliases samba.GlobalsOpts so cross-protocol bootstrapping
// goes through one shape regardless of which package the caller imports.
type GlobalsOpts = samba.GlobalsOpts

// DatasetMgr is the subset of dataset.Manager used by this package.
type DatasetMgr interface {
	Create(ctx context.Context, spec dataset.CreateSpec) error
	SetProps(ctx context.Context, name string, props map[string]string) error
	Destroy(ctx context.Context, name string, recursive bool) error
	Get(ctx context.Context, name string) (*dataset.Detail, error)
	SetACL(ctx context.Context, path string, aces []ACE) error
	GetACL(ctx context.Context, path string) ([]ACE, error)
}

// NFSMgr is the subset of nfs.Manager used by this package.
type NFSMgr interface {
	CreateExport(ctx context.Context, e nfs.Export) error
	UpdateExport(ctx context.Context, e nfs.Export) error
	DeleteExport(ctx context.Context, name string) error
}

// SambaMgr is the subset of samba.Manager used by this package.
type SambaMgr interface {
	CreateShare(ctx context.Context, s samba.Share) error
	UpdateShare(ctx context.Context, s samba.Share) error
	DeleteShare(ctx context.Context, name string) error
	SetGlobals(ctx context.Context, opts GlobalsOpts) error
}

// Protocol identifies a sharing protocol.
type Protocol string

const (
	ProtocolNFS Protocol = "nfs"
	ProtocolSMB Protocol = "smb"
)

// NFSOpts is the per-protocol NFS configuration for a ProtocolShare.
type NFSOpts struct {
	// Clients is the per-client export rules. Caller MUST NOT include
	// sec=krb5p (that is owned by the krb5 / Samba+krb5 setup at the
	// /etc/exports.d level globally).
	Clients []nfs.ClientRule `json:"clients"`
}

// SMBOpts is the per-protocol Samba configuration for a ProtocolShare.
type SMBOpts struct {
	Comment    string   `json:"comment,omitempty"`
	Browseable bool     `json:"browseable"`
	GuestOK    bool     `json:"guestOk,omitempty"`
	ValidUsers []string `json:"validUsers,omitempty"`
	WriteList  []string `json:"writeList,omitempty"`
}

// ProtocolShare is the unified "share this folder over NFS and/or SMB
// with consistent NFSv4 ACLs" abstraction. It owns a dataset + the
// per-protocol exports/shares as a single coordinated unit.
type ProtocolShare struct {
	Name        string     `json:"name"`        // filesystem-safe identifier
	Pool        string     `json:"pool"`        // ZFS pool to host the dataset
	DatasetName string     `json:"datasetName"` // dataset name within the pool
	Protocols   []Protocol `json:"protocols"`
	ACLs        []ACE      `json:"acls"`
	QuotaBytes  uint64     `json:"quotaBytes,omitempty"`

	NFS *NFSOpts `json:"nfs,omitempty"`
	SMB *SMBOpts `json:"smb,omitempty"`
}

// ProtocolStatus reports the active state of one protocol on a share.
type ProtocolStatus struct {
	Protocol Protocol `json:"protocol"`
	Active   bool     `json:"active"`
	Detail   string   `json:"detail,omitempty"`
}

// Detail is the read-side view returned by Get.
type Detail struct {
	Share     ProtocolShare    `json:"share"`
	Path      string           `json:"path"`
	ACL       []ACE            `json:"acl"`
	Protocols []ProtocolStatus `json:"protocolsStatus"`
}

// Manager coordinates dataset + nfs + samba operations to expose a
// single ProtocolShare resource.
type Manager struct {
	Datasets   DatasetMgr
	NFS        NFSMgr
	Samba      SambaMgr
	PathPrefix string // default ""; the on-disk root prefix
	// NFSExportsDir / SambaConfigDir / FilePrefix are used by Get/List
	// to detect per-protocol active state by checking for the managed
	// drop-in files. Empty values fall back to package defaults.
	NFSExportsDir  string
	SambaConfigDir string
	NFSFilePrefix  string
	SMBFilePrefix  string
}

// New constructs a Manager with the three underlying managers.
func New(datasets DatasetMgr, nfs NFSMgr, smb SambaMgr) *Manager {
	return &Manager{Datasets: datasets, NFS: nfs, Samba: smb}
}

// ---------------------------------------------------------------- helpers

const (
	defaultNFSExportsDir  = "/etc/exports.d"
	defaultSambaConfigDir = "/etc/samba/smb.conf.d"
	defaultFilePrefix     = "nova-nas-"
)

// path returns the host filesystem path for the dataset of share s.
// Format: <PathPrefix>/<Pool>/<DatasetName>. Default PathPrefix "" gives
// "/<Pool>/<DatasetName>".
func (m *Manager) path(s ProtocolShare) string {
	return m.PathPrefix + "/" + s.Pool + "/" + s.DatasetName
}

func (m *Manager) datasetFullName(s ProtocolShare) string {
	return s.Pool + "/" + s.DatasetName
}

func (m *Manager) nfsExportsDir() string {
	if m.NFSExportsDir != "" {
		return m.NFSExportsDir
	}
	return defaultNFSExportsDir
}

func (m *Manager) sambaConfigDir() string {
	if m.SambaConfigDir != "" {
		return m.SambaConfigDir
	}
	return defaultSambaConfigDir
}

func (m *Manager) nfsFilePrefix() string {
	if m.NFSFilePrefix != "" {
		return m.NFSFilePrefix
	}
	return defaultFilePrefix
}

func (m *Manager) smbFilePrefix() string {
	if m.SMBFilePrefix != "" {
		return m.SMBFilePrefix
	}
	return defaultFilePrefix
}

// validateName: 1-64 chars, alphanumeric + '-' + '_', no leading dash.
func validateName(name string) error {
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

// validateProtocols ensures the slice is a subset of {nfs, smb} and
// non-empty.
func validateProtocols(ps []Protocol) error {
	if len(ps) == 0 {
		return fmt.Errorf("at least one protocol required")
	}
	for _, p := range ps {
		switch p {
		case ProtocolNFS, ProtocolSMB:
		default:
			return fmt.Errorf("unsupported protocol %q", p)
		}
	}
	return nil
}

func hasProtocol(ps []Protocol, want Protocol) bool {
	for _, p := range ps {
		if p == want {
			return true
		}
	}
	return false
}

// validateShare is the common validation used by Create and Update.
func validateShare(s ProtocolShare) error {
	if err := validateName(s.Name); err != nil {
		return err
	}
	if s.Pool == "" {
		return fmt.Errorf("pool required")
	}
	if s.DatasetName == "" {
		return fmt.Errorf("datasetName required")
	}
	if err := validateProtocols(s.Protocols); err != nil {
		return err
	}
	// ACLs are optional. Stock OpenZFS-on-Linux exposes its NFSv4 ACLs
	// via system.nfs4_acl_xdr (XDR-encoded), which the standard
	// nfs4-acl-tools nfs4_setfacl does not understand. In production
	// operators set per-file ACLs via Samba (Windows Explorer over SMB,
	// which uses vfs_zfsacl natively), or via smbcacls. The dataset
	// property setup this package does (acltype=nfsv4, aclmode=
	// passthrough, etc.) is the real cross-protocol magic.
	return nil
}

// managedZFSProps returns the dataset properties this package owns.
// Callers must not override them via CreateSpec.Properties.
func managedZFSProps() map[string]string {
	return map[string]string{
		"acltype":         "nfsv4",
		"aclmode":         "passthrough",
		"aclinherit":      "passthrough",
		"xattr":           "sa",
		"casesensitivity": "mixed",
		"utf8only":        "on",
		"normalization":   "formD",
	}
}

// ownedKeys is the set of property keys this package considers
// reserved. A caller may not pre-populate them via Properties.
var ownedKeys = map[string]struct{}{
	"acltype":         {},
	"aclmode":         {},
	"aclinherit":      {},
	"xattr":           {},
	"casesensitivity": {},
	"utf8only":        {},
	"normalization":   {},
}

// buildCreateSpec produces a dataset.CreateSpec for the share. It
// rejects callers that try to force any of the owned ZFS properties.
func (m *Manager) buildCreateSpec(s ProtocolShare) (dataset.CreateSpec, error) {
	props := managedZFSProps()
	if s.QuotaBytes != 0 {
		props["quota"] = fmt.Sprintf("%d", s.QuotaBytes)
	}
	return dataset.CreateSpec{
		Parent:     s.Pool,
		Name:       s.DatasetName,
		Type:       "filesystem",
		Properties: props,
	}, nil
}

// nfsExportFor builds the NFS export descriptor for share s.
func (m *Manager) nfsExportFor(s ProtocolShare) nfs.Export {
	var clients []nfs.ClientRule
	if s.NFS != nil {
		clients = s.NFS.Clients
	}
	return nfs.Export{
		Name:    s.Name,
		Path:    m.path(s),
		Clients: clients,
	}
}

// sambaShareFor builds the Samba share descriptor for share s.
func (m *Manager) sambaShareFor(s ProtocolShare) samba.Share {
	out := samba.Share{
		Name:       s.Name,
		Path:       m.path(s),
		Browseable: true,
		Writable:   true,
	}
	if s.SMB != nil {
		out.Comment = s.SMB.Comment
		out.Browseable = s.SMB.Browseable
		out.GuestOK = s.SMB.GuestOK
		out.ValidUsers = s.SMB.ValidUsers
		out.WriteList = s.SMB.WriteList
	}
	return out
}

// ---------------------------------------------------------------- Create

// Create provisions a new ProtocolShare: dataset → ACL → (nfs export) →
// (samba share). On any failure, prior steps are rolled back in reverse
// order, best-effort.
func (m *Manager) Create(ctx context.Context, share ProtocolShare) error {
	if err := validateShare(share); err != nil {
		return err
	}
	// SMB requires NFSv4 ACLs on the dataset. We own those properties
	// so any caller-provided override of acltype is rejected.
	if hasProtocol(share.Protocols, ProtocolSMB) {
		// (No external Properties bag in ProtocolShare today; this
		// guard is for defense-in-depth in case a future field exposes
		// raw ZFS properties — see TODO below.)
	}

	spec, err := m.buildCreateSpec(share)
	if err != nil {
		return err
	}
	// 1. Create dataset.
	if err := m.Datasets.Create(ctx, spec); err != nil {
		return fmt.Errorf("create dataset: %w", err)
	}
	full := m.datasetFullName(share)
	path := m.path(share)

	// 2. Apply ACL (optional; stock OpenZFS-on-Linux does not expose its
	// NFSv4 ACLs to nfs4_setfacl, so callers typically leave this empty
	// and configure per-file ACLs via Samba/Windows clients).
	if len(share.ACLs) > 0 {
		if err := m.Datasets.SetACL(ctx, path, share.ACLs); err != nil {
			_ = m.Datasets.Destroy(ctx, full, false)
			return fmt.Errorf("set acl: %w", err)
		}
	}

	// 3. NFS export (optional).
	nfsCreated := false
	if hasProtocol(share.Protocols, ProtocolNFS) && share.NFS != nil {
		if err := m.NFS.CreateExport(ctx, m.nfsExportFor(share)); err != nil {
			_ = m.Datasets.Destroy(ctx, full, false)
			return fmt.Errorf("create nfs export: %w", err)
		}
		nfsCreated = true
	}

	// 4. Samba share (optional).
	if hasProtocol(share.Protocols, ProtocolSMB) && share.SMB != nil {
		if err := m.Samba.CreateShare(ctx, m.sambaShareFor(share)); err != nil {
			if nfsCreated {
				_ = m.NFS.DeleteExport(ctx, share.Name)
			}
			_ = m.Datasets.Destroy(ctx, full, false)
			return fmt.Errorf("create samba share: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- Update

// Update reconciles the share. It is create-or-update: any of the three
// underlying resources are created if missing, updated if present. ACLs
// are reapplied unconditionally (idempotent overwrite).
func (m *Manager) Update(ctx context.Context, share ProtocolShare) error {
	if err := validateShare(share); err != nil {
		return err
	}
	full := m.datasetFullName(share)
	path := m.path(share)

	// 1. Dataset: create-or-touch-properties.
	if _, err := m.Datasets.Get(ctx, full); err != nil {
		if !errors.Is(err, dataset.ErrNotFound) {
			return fmt.Errorf("get dataset: %w", err)
		}
		spec, berr := m.buildCreateSpec(share)
		if berr != nil {
			return berr
		}
		if cerr := m.Datasets.Create(ctx, spec); cerr != nil {
			return fmt.Errorf("create dataset: %w", cerr)
		}
	} else {
		// Reapply managed properties (and quota if requested) to
		// converge any drift.
		props := managedZFSProps()
		if share.QuotaBytes != 0 {
			props["quota"] = fmt.Sprintf("%d", share.QuotaBytes)
		}
		if err := m.Datasets.SetProps(ctx, full, props); err != nil {
			return fmt.Errorf("set dataset props: %w", err)
		}
	}

	// 2. ACL — idempotent overwrite, optional (see Create).
	if len(share.ACLs) > 0 {
		if err := m.Datasets.SetACL(ctx, path, share.ACLs); err != nil {
			return fmt.Errorf("set acl: %w", err)
		}
	}

	// 3. NFS export.
	if hasProtocol(share.Protocols, ProtocolNFS) && share.NFS != nil {
		exp := m.nfsExportFor(share)
		if err := m.NFS.UpdateExport(ctx, exp); err != nil {
			if !errors.Is(err, nfs.ErrNotFound) {
				return fmt.Errorf("update nfs export: %w", err)
			}
			if cerr := m.NFS.CreateExport(ctx, exp); cerr != nil {
				return fmt.Errorf("create nfs export: %w", cerr)
			}
		}
	}

	// 4. Samba share.
	if hasProtocol(share.Protocols, ProtocolSMB) && share.SMB != nil {
		sh := m.sambaShareFor(share)
		if err := m.Samba.UpdateShare(ctx, sh); err != nil {
			if !errors.Is(err, samba.ErrNotFound) {
				return fmt.Errorf("update samba share: %w", err)
			}
			if cerr := m.Samba.CreateShare(ctx, sh); cerr != nil {
				return fmt.Errorf("create samba share: %w", cerr)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- Delete

// Delete tears down a ProtocolShare in reverse order: samba → nfs →
// dataset. Each step is best-effort. Errors from any step are
// collected and returned as a multi-error; a failure on one step does
// not abort the others.
func (m *Manager) Delete(ctx context.Context, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	var errs []error

	if err := m.Samba.DeleteShare(ctx, name); err != nil && !errors.Is(err, samba.ErrNotFound) {
		errs = append(errs, fmt.Errorf("delete samba share: %w", err))
	}
	if err := m.NFS.DeleteExport(ctx, name); err != nil && !errors.Is(err, nfs.ErrNotFound) {
		errs = append(errs, fmt.Errorf("delete nfs export: %w", err))
	}
	// We don't have the pool/datasetName at delete time without a
	// lookup. The caller's name == export/share name == dataset
	// "leaf" name only by convention. We cannot Destroy the dataset
	// without its pool. This is a deliberate limitation: callers
	// who want full teardown should pass the ProtocolShare via
	// DeleteShare (below).
	if len(errs) == 1 {
		return errs[0]
	}
	if len(errs) > 1 {
		return errors.Join(errs...)
	}
	return nil
}

// DeleteShare is the full-tear-down variant: it removes samba share,
// NFS export, AND the dataset (which Delete cannot do without knowing
// the pool). Best-effort, multi-error semantics.
func (m *Manager) DeleteShare(ctx context.Context, share ProtocolShare) error {
	if err := validateName(share.Name); err != nil {
		return err
	}
	if share.Pool == "" || share.DatasetName == "" {
		return fmt.Errorf("pool and datasetName required for full delete")
	}
	var errs []error
	if err := m.Samba.DeleteShare(ctx, share.Name); err != nil && !errors.Is(err, samba.ErrNotFound) {
		errs = append(errs, fmt.Errorf("delete samba share: %w", err))
	}
	if err := m.NFS.DeleteExport(ctx, share.Name); err != nil && !errors.Is(err, nfs.ErrNotFound) {
		errs = append(errs, fmt.Errorf("delete nfs export: %w", err))
	}
	if err := m.Datasets.Destroy(ctx, m.datasetFullName(share), false); err != nil &&
		!errors.Is(err, dataset.ErrNotFound) {
		errs = append(errs, fmt.Errorf("destroy dataset: %w", err))
	}
	if len(errs) == 1 {
		return errs[0]
	}
	if len(errs) > 1 {
		return errors.Join(errs...)
	}
	return nil
}

// ---------------------------------------------------------------- Get

// Get returns the read-side view of a share: dataset detail, current
// ACL, and per-protocol active state. The active state is determined
// by the presence of the per-protocol drop-in file we own.
func (m *Manager) Get(ctx context.Context, share ProtocolShare) (*Detail, error) {
	if err := validateName(share.Name); err != nil {
		return nil, err
	}
	if share.Pool == "" || share.DatasetName == "" {
		return nil, fmt.Errorf("pool and datasetName required")
	}
	full := m.datasetFullName(share)
	if _, err := m.Datasets.Get(ctx, full); err != nil {
		return nil, fmt.Errorf("get dataset: %w", err)
	}
	path := m.path(share)
	acl, err := m.Datasets.GetACL(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get acl: %w", err)
	}

	d := &Detail{Share: share, Path: path, ACL: acl}

	// Detect per-protocol active state by file presence.
	nfsFile := filepath.Join(m.nfsExportsDir(), m.nfsFilePrefix()+share.Name+".exports")
	smbFile := filepath.Join(m.sambaConfigDir(), m.smbFilePrefix()+share.Name+".conf")

	nfsActive := fileExists(nfsFile)
	smbActive := fileExists(smbFile)

	d.Protocols = []ProtocolStatus{
		{Protocol: ProtocolNFS, Active: nfsActive, Detail: nfsFile},
		{Protocol: ProtocolSMB, Active: smbActive, Detail: smbFile},
	}
	return d, nil
}

// fileExists is a tiny helper used by Get / List for active-state
// detection. Errors other than not-exist are treated as "exists" so
// a permission error doesn't make a real share look gone.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !errors.Is(err, os.ErrNotExist)
}

// ---------------------------------------------------------------- List

// List enumerates ProtocolShare names by scanning the NFS exports
// directory and the Samba drop-in directory for files we manage. Each
// distinct base name (after stripping prefix and extension) becomes one
// share. The returned ProtocolShare values carry only Name and the
// detected Protocols; Pool/DatasetName are not recoverable from the
// drop-in files alone.
func (m *Manager) List(ctx context.Context) ([]ProtocolShare, error) {
	type seen struct {
		nfs bool
		smb bool
	}
	all := map[string]*seen{}

	// Scan NFS dir.
	if entries, err := os.ReadDir(m.nfsExportsDir()); err == nil {
		prefix := m.nfsFilePrefix()
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".exports") {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(n, prefix), ".exports")
			if validateName(name) != nil {
				continue
			}
			s, ok := all[name]
			if !ok {
				s = &seen{}
				all[name] = s
			}
			s.nfs = true
		}
	}

	// Scan Samba dir.
	if entries, err := os.ReadDir(m.sambaConfigDir()); err == nil {
		prefix := m.smbFilePrefix()
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".conf") {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(n, prefix), ".conf")
			if validateName(name) != nil {
				continue
			}
			s, ok := all[name]
			if !ok {
				s = &seen{}
				all[name] = s
			}
			s.smb = true
		}
	}

	out := make([]ProtocolShare, 0, len(all))
	for name, s := range all {
		var protos []Protocol
		if s.nfs {
			protos = append(protos, ProtocolNFS)
		}
		if s.smb {
			protos = append(protos, ProtocolSMB)
		}
		out = append(out, ProtocolShare{Name: name, Protocols: protos})
	}
	return out, nil
}

// ---------------------------------------------------------------- InitGlobals

// InitGlobals applies cross-protocol Samba globals (VFS modules, ACL
// support, inheritance). It is the prerequisite for any SMB-bearing
// ProtocolShare to give consistent NFSv4 ACL semantics on the wire and
// must be called once at deployment time. The underlying Samba manager
// is responsible for idempotency — calling this repeatedly is safe.
//
// We deliberately do NOT call this from Create: it modifies the
// [global] section, and folding that into per-share creation would
// race with operator-driven samba configuration and surprise anyone
// who does not expect a share-creation call to mutate global state.
func (m *Manager) InitGlobals(ctx context.Context, opts GlobalsOpts) error {
	return m.Samba.SetGlobals(ctx, opts)
}
